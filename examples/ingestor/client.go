package main

import (
	"net/http"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// newClient builds an opensearch-go client sized for many parallel workers.
// http.DefaultTransport keeps only two idle connections per host, so with
// sixteen workers most bulk requests would open a fresh TCP (and TLS)
// connection. Give every worker a connection of its own.
// #region client
func newClient(addr, user, pass string, workers int) (*opensearchapi.Client, error) {
	tp := http.DefaultTransport.(*http.Transport).Clone()
	tp.MaxIdleConnsPerHost = workers
	tp.MaxConnsPerHost = workers + 2

	return opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses:          []string{addr},
			Username:           user,
			Password:           pass,
			Transport:          tp,
			InsecureSkipVerify: true,                 // demo cluster, self-signed certificate
			RetryOnStatus:      []int{502, 503, 504}, // whole-request retries by the transport
			MaxRetries:         3,                    // per-document 429s are handled by us
		},
	})
}

// #endregion client
