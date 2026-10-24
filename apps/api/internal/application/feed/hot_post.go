package applicationfeed

import (
	"context"
	domainpost "social/internal/domain/post"
)

type HotPostRepository interface {
	ListHotPostCandidates(ctx context.Context, limit int, offset int) ([]*domainpost.HotItem, error)
	BatchGetPublicFeedItems(ctx context.Context, viewerID int64, postIDs []int64) (map[int64]*domainpost.FeedItem, error)
}

type HotPostCache interface {
	ListPostHotWindow(ctx context.Context, offset int, limit int) ([]*domainpost.HotItem, error)
	BatchGetPostStats(ctx context.Context, postIDs []int64, fallbacks map[int64]*domainpost.Stat) (map[int64]*domainpost.Stat, error)
}

type HotPostService struct {
	repo  HotPostRepository
	cache HotPostCache
}

type HotPostResult struct {
	Items []*domainpost.FeedItem
}

func NewHotPostService(repo HotPostRepository, cache HotPostCache) *HotPostService {
	return &HotPostService{repo: repo, cache: cache}
}

func (s *HotPostService) List(ctx context.Context, viewerID int64, limit int, offset int) (*HotPostResult, error) {
	var candidates []*domainpost.HotItem
	if s.cache != nil {
		cached, err := s.cache.ListPostHotWindow(ctx, offset, limit)
		if err == nil {
			candidates = cached
		}
	}
	if len(candidates) == 0 {
		loaded, err := s.repo.ListHotPostCandidates(ctx, limit, offset)
		if err != nil {
			return nil, ErrLoadFollowingPostFeedFailed
		}
		candidates = loaded
	}
	postIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		postIDs = append(postIDs, candidate.PostID)
	}
	itemsByID, err := s.repo.BatchGetPublicFeedItems(ctx, viewerID, postIDs)
	if err != nil {
		return nil, ErrLoadFollowingPostFeedFailed
	}
	if s.cache != nil {
		if stats, statErr := s.cache.BatchGetPostStats(ctx, postIDs, feedItemStats(itemsByID)); statErr == nil {
			overlayPostStats(itemsByID, stats)
		}
	}
	items := make([]*domainpost.FeedItem, 0, len(postIDs))
	for _, postID := range postIDs {
		if item := itemsByID[postID]; item != nil {
			items = append(items, item)
		}
	}
	return &HotPostResult{Items: items}, nil
}
