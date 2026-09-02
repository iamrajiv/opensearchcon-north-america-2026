package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// prepareIndex recreates the index tuned for a bulk load: no replicas to
// copy to, and no refresh so OpenSearch is not cutting a new segment every
// second while we write.
// #region prepare
func prepareIndex(ctx context.Context, client *opensearchapi.Client, index string, shards int) error {
	_, _ = client.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{Indices: []string{index}})
	body := fmt.Sprintf(`{"settings":{"index":{
		"number_of_shards":%d, "number_of_replicas":0, "refresh_interval":"-1"}}}`, shards)
	_, err := client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: index, Body: strings.NewReader(body),
	})
	return err
}

// #endregion prepare

// finishIndex turns refresh back on, makes everything searchable, and
// returns the number of documents OpenSearch actually holds.
func finishIndex(ctx context.Context, client *opensearchapi.Client, index string) (int, error) {
	_, err := client.Indices.Settings.Put(ctx, opensearchapi.SettingsPutReq{
		Indices: []string{index}, Body: strings.NewReader(`{"index":{"refresh_interval":"1s"}}`),
	})
	if err != nil {
		return 0, err
	}
	if _, err := client.Indices.Refresh(ctx, &opensearchapi.IndicesRefreshReq{Index: []string{index}}); err != nil {
		return 0, err
	}
	resp, err := client.Indices.Count(ctx, &opensearchapi.IndicesCountReq{Indices: []string{index}})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}
