package cache_test

import (
	"errors"
	"testing"

	"k8s-scout/internal/cache"
	"k8s-scout/internal/issue"
)

func TestGetSetCopy(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "one", Repository: "kubernetes-sigs/kind"}}, nil)

	got, updatedAt, err := c.Get()
	if err != nil || updatedAt.IsZero() || len(got) != 1 {
		t.Fatalf("Get = %v %v %v", got, updatedAt, err)
	}
	got[0].Title = "mutated"
	again, _, _ := c.Get()
	if again[0].Title != "one" {
		t.Fatal("Get should return a copy")
	}

	h := c.HealthSnapshot()
	if h.Status != "ok" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}

func TestHealthDegraded(t *testing.T) {
	c := &cache.Cache{}
	c.Set([]issue.Issue{{Title: "one"}}, nil)
	c.Set(nil, errors.New("boom"))
	h := c.HealthSnapshot()
	if h.Status != "degraded" || h.Error != "boom" || h.Issues != 1 {
		t.Fatalf("health = %+v", h)
	}
}
