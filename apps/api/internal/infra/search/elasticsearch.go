package infrasearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraconfig "social/internal/infra/config"
)

var ErrInvalidElasticsearchConfig = errors.New("invalid elasticsearch config")

type Client struct {
	baseURL string
	index   string
	http    *http.Client
}

type document struct {
	PostID      int64  `json:"post_id"`
	Content     string `json:"content"`
	PublishedAt int64  `json:"published_at"`
}

func New(cfg infraconfig.SearchConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	index := strings.TrimSpace(cfg.Index)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || index == "" || strings.Contains(index, "/") {
		return nil, ErrInvalidElasticsearchConfig
	}
	return &Client{baseURL: baseURL, index: index, http: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (c *Client) EnsureIndex(ctx context.Context) error {
	response, err := c.request(ctx, http.MethodHead, "/"+url.PathEscape(c.index), nil)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusOK {
		_ = response.Body.Close()
		return nil
	}
	if response.StatusCode != http.StatusNotFound {
		defer response.Body.Close()
		return responseError(response)
	}
	_ = response.Body.Close()
	body := map[string]any{
		"mappings": map[string]any{
			"dynamic": "strict",
			"properties": map[string]any{
				"post_id":      map[string]any{"type": "long"},
				"content":      map[string]any{"type": "text", "analyzer": "cjk", "search_analyzer": "cjk"},
				"published_at": map[string]any{"type": "date", "format": "epoch_millis"},
			},
		},
	}
	response, err = c.request(ctx, http.MethodPut, "/"+url.PathEscape(c.index), body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		createErr := responseError(response)
		exists, checkErr := c.indexExists(ctx)
		if checkErr == nil && exists {
			return nil
		}
		return createErr
	}
	return nil
}

func (c *Client) indexExists(ctx context.Context) (bool, error) {
	response, err := c.request(ctx, http.MethodHead, "/"+url.PathEscape(c.index), nil)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return true, nil
	}
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, responseError(response)
}

func (c *Client) UpsertPost(ctx context.Context, postID int64, content string, publishedAt int64) error {
	response, err := c.request(ctx, http.MethodPut, c.documentPath(postID), document{PostID: postID, Content: content, PublishedAt: publishedAt})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return responseError(response)
	}
	return nil
}

func (c *Client) DeletePost(ctx context.Context, postID int64) error {
	response, err := c.request(ctx, http.MethodDelete, c.documentPath(postID), nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		return responseError(response)
	}
	return nil
}

func (c *Client) SearchPostIDs(ctx context.Context, keyword string, limit int, offset int) ([]int64, error) {
	body := map[string]any{
		"from":    offset,
		"size":    limit,
		"_source": false,
		"query": map[string]any{
			"match": map[string]any{"content": map[string]any{"query": keyword}},
		},
		"sort": []any{
			map[string]any{"_score": map[string]any{"order": "desc"}},
			map[string]any{"published_at": map[string]any{"order": "desc"}},
			map[string]any{"post_id": map[string]any{"order": "desc"}},
		},
	}
	response, err := c.request(ctx, http.MethodPost, "/"+url.PathEscape(c.index)+"/_search", body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseError(response)
	}
	var result struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	postIDs := make([]int64, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		postID, err := strconv.ParseInt(hit.ID, 10, 64)
		if err != nil {
			return nil, err
		}
		postIDs = append(postIDs, postID)
	}
	return postIDs, nil
}

func (c *Client) documentPath(postID int64) string {
	return "/" + url.PathEscape(c.index) + "/_doc/" + strconv.FormatInt(postID, 10)
}

func (c *Client) request(ctx context.Context, method string, path string, body any) (*http.Response, error) {
	var content io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		content = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, content)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(request)
}

func responseError(response *http.Response) error {
	content, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return fmt.Errorf("elasticsearch returned %s: %s", response.Status, strings.TrimSpace(string(content)))
}
