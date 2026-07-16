package tools

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

func TestPortalClientCacheTTL(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	loadCount := 0
	cache := &portalClientCache{
		ttl: portalClientTTL,
		now: func() time.Time { return now },
		load: func(context.Context) (*portal.PortalClient, error) {
			loadCount++
			return &portal.PortalClient{}, nil
		},
	}

	first, err := cache.get(context.Background())
	if err != nil {
		t.Fatalf("first get: %v", err)
	}

	now = now.Add(29*time.Minute + 59*time.Second)
	beforeExpiry, err := cache.get(context.Background())
	if err != nil {
		t.Fatalf("get before expiry: %v", err)
	}
	if beforeExpiry != first {
		t.Fatal("get before expiry returned a new client")
	}

	now = now.Add(time.Second)
	atExpiry, err := cache.get(context.Background())
	if err != nil {
		t.Fatalf("get at expiry: %v", err)
	}
	if atExpiry == first {
		t.Fatal("get at expiry returned the expired client")
	}
	if loadCount != 2 {
		t.Fatalf("load count = %d, want 2", loadCount)
	}
}

func TestPortalClientCacheRetriesAfterRefreshFailure(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	loadCount := 0
	cache := &portalClientCache{
		ttl: portalClientTTL,
		now: func() time.Time { return now },
		load: func(context.Context) (*portal.PortalClient, error) {
			loadCount++
			if loadCount == 2 {
				return nil, errors.New("login failed")
			}
			return &portal.PortalClient{}, nil
		},
	}

	first, err := cache.get(context.Background())
	if err != nil {
		t.Fatalf("first get: %v", err)
	}

	now = now.Add(30 * time.Minute)
	if _, err := cache.get(context.Background()); err == nil {
		t.Fatal("refresh succeeded, want login error")
	}

	refreshed, err := cache.get(context.Background())
	if err != nil {
		t.Fatalf("retry refresh: %v", err)
	}
	if refreshed == first {
		t.Fatal("retry refresh returned the expired client")
	}
	if loadCount != 3 {
		t.Fatalf("load count = %d, want 3", loadCount)
	}
}

func TestPortalClientCacheLoadsOnceForConcurrentCalls(t *testing.T) {
	var loadCount atomic.Int32
	client := &portal.PortalClient{}
	cache := &portalClientCache{
		ttl: portalClientTTL,
		now: time.Now,
		load: func(context.Context) (*portal.PortalClient, error) {
			loadCount.Add(1)
			return client, nil
		},
	}

	const callers = 20
	start := make(chan struct{})
	results := make(chan *portal.PortalClient, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := cache.get(context.Background())
			results <- got
			errs <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent get: %v", err)
		}
	}
	for got := range results {
		if got != client {
			t.Fatal("concurrent get returned an unexpected client")
		}
	}
	if got := loadCount.Load(); got != 1 {
		t.Fatalf("load count = %d, want 1", got)
	}
}
