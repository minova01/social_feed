package applicationfeed

import (
	"context"
	"errors"
	"math/rand"
	domainpost "social/internal/domain/post"
	"sort"
	"time"
)

const followingSourceLimit = 50
const followingFallbackLimit = 10

var ErrLoadFollowingPostFeedFailed = errors.New("failed to load following post feed")
var ErrUpdateFollowingPostFeedStateFailed = errors.New("failed to update following post feed state")

type FollowingPostRepository interface {
	GetFeedState(ctx context.Context, userID int64) (*domainpost.FeedState, error)
	SaveFeedState(ctx context.Context, state *domainpost.FeedState) error
	ListFollowingBigCreatorIDs(ctx context.Context, viewerID int64) ([]int64, error)
	ListFollowingPostWindow(ctx context.Context, viewerID int64, bigCreator bool, after domainpost.TimelineCursor, cutoffAt int64, limit int) (*domainpost.TimelineBatch, error)
	ListFollowingRecentPosts(ctx context.Context, viewerID int64, bigCreator bool, cutoffAt int64, limit int) ([]*domainpost.TimelineItem, error)
	BatchGetFollowingFeedItems(ctx context.Context, viewerID int64, postIDs []int64) (map[int64]*domainpost.FeedItem, error)
}

type FollowingPostCache interface {
	GetPostFeedRemainder(ctx context.Context, userID int64) (*domainpost.FeedRemainder, bool, error)
	SetPostFeedRemainder(ctx context.Context, userID int64, remainder *domainpost.FeedRemainder) error
	DeletePostFeedRemainder(ctx context.Context, userID int64) error
	ListPostFollowingWindow(ctx context.Context, viewerID int64, authorIDs []int64, after domainpost.TimelineCursor, cutoffAt int64, limit int) (*domainpost.TimelineBatch, *domainpost.TimelineBatch, bool, error)
	ListPostFollowingRecent(ctx context.Context, viewerID int64, authorIDs []int64, cutoffAt int64, limit int) ([]*domainpost.TimelineItem, []*domainpost.TimelineItem, bool, error)
	BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error)
}

type FollowingPostService struct {
	repo  FollowingPostRepository
	cache FollowingPostCache
}

type FollowingPostResult struct {
	Items []*domainpost.FeedItem
}

type followingCandidate struct {
	timeline *domainpost.TimelineItem
	item     *domainpost.FeedItem
}

func NewFollowingPostService(repo FollowingPostRepository, cache FollowingPostCache) *FollowingPostService {
	return &FollowingPostService{repo: repo, cache: cache}
}

func (s *FollowingPostService) List(ctx context.Context, viewerID int64, limit int) (*FollowingPostResult, error) {
	cacheHealthy := s.cache != nil
	if cacheHealthy {
		remainder, ok, err := s.cache.GetPostFeedRemainder(ctx, viewerID)
		if err != nil {
			cacheHealthy = false
		} else if ok {
			result, consumed, err := s.consumeRemainder(ctx, viewerID, limit, remainder)
			if err != nil || consumed {
				return result, err
			}
		}
	}

	state, err := s.repo.GetFeedState(ctx, viewerID)
	if err != nil {
		return nil, ErrLoadFollowingPostFeedFailed
	}
	cutoffAt := time.Now().UnixMilli()
	after := domainpost.TimelineCursor{PublishedAt: state.LastSeenAt, PostID: state.LastSeenPostID}
	authorIDs, err := s.repo.ListFollowingBigCreatorIDs(ctx, viewerID)
	if err != nil {
		return nil, ErrLoadFollowingPostFeedFailed
	}

	inbox, outbox, loadedFromCache, err := s.loadWindow(ctx, viewerID, authorIDs, after, cutoffAt, cacheHealthy)
	if err != nil {
		return nil, err
	}
	cacheHealthy = cacheHealthy && loadedFromCache
	safeWatermark := safeFollowingWatermark(inbox, outbox, cutoffAt)
	candidates, err := s.loadCandidates(ctx, viewerID, mergeTimelineItems(inbox.Items, outbox.Items, &safeWatermark))
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return s.listFallback(ctx, viewerID, authorIDs, cutoffAt, limit, cacheHealthy)
	}

	if len(candidates) > limit {
		if cacheHealthy {
			shuffleCandidates(candidates)
			if err := s.cache.SetPostFeedRemainder(ctx, viewerID, &domainpost.FeedRemainder{
				Items:         candidateTimelines(candidates[limit:]),
				SafeWatermark: safeWatermark,
			}); err == nil {
				return &FollowingPostResult{Items: candidateItems(candidates[:limit])}, nil
			}
		}
		sortCandidates(candidates)
		safeWatermark = timelineCursor(candidates[limit-1].timeline)
		candidates = candidates[:limit]
	}
	shuffleCandidates(candidates)
	if err := s.saveWatermark(ctx, viewerID, safeWatermark); err != nil {
		return nil, err
	}
	return &FollowingPostResult{Items: candidateItems(candidates)}, nil
}

func (s *FollowingPostService) consumeRemainder(ctx context.Context, viewerID int64, limit int, remainder *domainpost.FeedRemainder) (*FollowingPostResult, bool, error) {
	candidates, err := s.loadCandidates(ctx, viewerID, remainder.Items)
	if err != nil {
		return nil, true, err
	}
	if len(candidates) == 0 {
		if err := s.saveWatermark(ctx, viewerID, remainder.SafeWatermark); err != nil {
			return nil, true, err
		}
		_ = s.cache.DeletePostFeedRemainder(ctx, viewerID)
		return nil, false, nil
	}
	shuffleCandidates(candidates)
	if len(candidates) > limit {
		_ = s.cache.SetPostFeedRemainder(ctx, viewerID, &domainpost.FeedRemainder{
			Items:         candidateTimelines(candidates[limit:]),
			SafeWatermark: remainder.SafeWatermark,
		})
		return &FollowingPostResult{Items: candidateItems(candidates[:limit])}, true, nil
	}
	if err := s.saveWatermark(ctx, viewerID, remainder.SafeWatermark); err != nil {
		return nil, true, err
	}
	_ = s.cache.DeletePostFeedRemainder(ctx, viewerID)
	return &FollowingPostResult{Items: candidateItems(candidates)}, true, nil
}

func (s *FollowingPostService) loadWindow(ctx context.Context, viewerID int64, authorIDs []int64, after domainpost.TimelineCursor, cutoffAt int64, useCache bool) (*domainpost.TimelineBatch, *domainpost.TimelineBatch, bool, error) {
	if useCache {
		inbox, outbox, hasIndex, err := s.cache.ListPostFollowingWindow(ctx, viewerID, authorIDs, after, cutoffAt, followingSourceLimit)
		if err == nil && hasIndex {
			return inbox, outbox, true, nil
		}
	}
	inbox, err := s.repo.ListFollowingPostWindow(ctx, viewerID, false, after, cutoffAt, followingSourceLimit)
	if err != nil {
		return nil, nil, false, ErrLoadFollowingPostFeedFailed
	}
	outbox, err := s.repo.ListFollowingPostWindow(ctx, viewerID, true, after, cutoffAt, followingSourceLimit)
	if err != nil {
		return nil, nil, false, ErrLoadFollowingPostFeedFailed
	}
	return inbox, outbox, false, nil
}

func (s *FollowingPostService) listFallback(ctx context.Context, viewerID int64, authorIDs []int64, cutoffAt int64, limit int, useCache bool) (*FollowingPostResult, error) {
	var inbox []*domainpost.TimelineItem
	var outbox []*domainpost.TimelineItem
	if useCache {
		cachedInbox, cachedOutbox, hasIndex, err := s.cache.ListPostFollowingRecent(ctx, viewerID, authorIDs, cutoffAt, followingFallbackLimit)
		if err == nil && hasIndex {
			inbox, outbox = cachedInbox, cachedOutbox
		} else {
			useCache = false
		}
	}
	if !useCache {
		var err error
		inbox, err = s.repo.ListFollowingRecentPosts(ctx, viewerID, false, cutoffAt, followingFallbackLimit)
		if err != nil {
			return nil, ErrLoadFollowingPostFeedFailed
		}
		outbox, err = s.repo.ListFollowingRecentPosts(ctx, viewerID, true, cutoffAt, followingFallbackLimit)
		if err != nil {
			return nil, ErrLoadFollowingPostFeedFailed
		}
	}
	candidates, err := s.loadCandidates(ctx, viewerID, mergeTimelineItems(inbox, outbox, nil))
	if err != nil {
		return nil, err
	}
	shuffleCandidates(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if err := s.saveWatermark(ctx, viewerID, cutoffWatermark(cutoffAt, inbox, outbox)); err != nil {
		return nil, err
	}
	return &FollowingPostResult{Items: candidateItems(candidates)}, nil
}

func (s *FollowingPostService) loadCandidates(ctx context.Context, viewerID int64, timelineItems []*domainpost.TimelineItem) ([]followingCandidate, error) {
	postIDs := make([]int64, 0, len(timelineItems))
	for _, item := range timelineItems {
		postIDs = append(postIDs, item.PostID)
	}
	items, err := s.repo.BatchGetFollowingFeedItems(ctx, viewerID, postIDs)
	if err != nil {
		return nil, ErrLoadFollowingPostFeedFailed
	}
	if s.cache != nil {
		if stats, statErr := s.cache.BatchGetPostStats(ctx, postIDs, feedItemStats(items)); statErr == nil {
			overlayPostStats(items, stats)
		}
	}
	candidates := make([]followingCandidate, 0, len(timelineItems))
	for _, timeline := range timelineItems {
		if item := items[timeline.PostID]; item != nil {
			candidates = append(candidates, followingCandidate{timeline: timeline, item: item})
		}
	}
	return candidates, nil
}

func feedItemStats(items map[int64]*domainpost.FeedItem) map[int64]*domainpost.Stat {
	stats := make(map[int64]*domainpost.Stat, len(items))
	for postID, item := range items {
		stats[postID] = &domainpost.Stat{PostID: postID, LikeCount: item.LikeCount, FavoriteCount: item.FavoriteCount}
	}
	return stats
}

func overlayPostStats(items map[int64]*domainpost.FeedItem, stats map[int64]*domainpost.Stat) {
	for postID, stat := range stats {
		if item := items[postID]; item != nil && stat != nil {
			item.LikeCount = stat.LikeCount
			item.FavoriteCount = stat.FavoriteCount
		}
	}
}

func (s *FollowingPostService) saveWatermark(ctx context.Context, viewerID int64, cursor domainpost.TimelineCursor) error {
	if err := s.repo.SaveFeedState(ctx, &domainpost.FeedState{
		UserID:         viewerID,
		LastSeenAt:     cursor.PublishedAt,
		LastSeenPostID: cursor.PostID,
	}); err != nil {
		return ErrUpdateFollowingPostFeedStateFailed
	}
	return nil
}

func safeFollowingWatermark(inbox *domainpost.TimelineBatch, outbox *domainpost.TimelineBatch, cutoffAt int64) domainpost.TimelineCursor {
	safe := cutoffWatermark(cutoffAt, inbox.Items, outbox.Items)
	for _, batch := range []*domainpost.TimelineBatch{inbox, outbox} {
		if batch.Capped && len(batch.Items) > 0 {
			boundary := timelineCursor(batch.Items[len(batch.Items)-1])
			if cursorBefore(boundary, safe) {
				safe = boundary
			}
		}
	}
	return safe
}

func cutoffWatermark(cutoffAt int64, sources ...[]*domainpost.TimelineItem) domainpost.TimelineCursor {
	cursor := domainpost.TimelineCursor{PublishedAt: cutoffAt}
	for _, items := range sources {
		for _, item := range items {
			if item != nil && item.PublishedAt == cutoffAt && item.PostID > cursor.PostID {
				cursor.PostID = item.PostID
			}
		}
	}
	return cursor
}

func mergeTimelineItems(inbox []*domainpost.TimelineItem, outbox []*domainpost.TimelineItem, boundary *domainpost.TimelineCursor) []*domainpost.TimelineItem {
	items := make([]*domainpost.TimelineItem, 0, len(inbox)+len(outbox))
	seen := map[int64]struct{}{}
	for _, item := range append(inbox, outbox...) {
		if item == nil || item.PostID <= 0 {
			continue
		}
		if boundary != nil && cursorBefore(*boundary, timelineCursor(item)) {
			continue
		}
		if _, ok := seen[item.PostID]; ok {
			continue
		}
		seen[item.PostID] = struct{}{}
		items = append(items, item)
	}
	return items
}

func timelineCursor(item *domainpost.TimelineItem) domainpost.TimelineCursor {
	return domainpost.TimelineCursor{PublishedAt: item.PublishedAt, PostID: item.PostID}
}

func cursorBefore(left domainpost.TimelineCursor, right domainpost.TimelineCursor) bool {
	return left.PublishedAt < right.PublishedAt || left.PublishedAt == right.PublishedAt && left.PostID < right.PostID
}

func sortCandidates(candidates []followingCandidate) {
	sort.Slice(candidates, func(i int, j int) bool {
		return cursorBefore(timelineCursor(candidates[i].timeline), timelineCursor(candidates[j].timeline))
	})
}

func shuffleCandidates(candidates []followingCandidate) {
	rand.Shuffle(len(candidates), func(i int, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
}

func candidateItems(candidates []followingCandidate) []*domainpost.FeedItem {
	items := make([]*domainpost.FeedItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, candidate.item)
	}
	return items
}

func candidateTimelines(candidates []followingCandidate) []*domainpost.TimelineItem {
	items := make([]*domainpost.TimelineItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, candidate.timeline)
	}
	return items
}
