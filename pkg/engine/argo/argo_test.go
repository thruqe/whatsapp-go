package argo

import (
	"testing"
)

func TestArgoStoreLoad(t *testing.T) {
	store, err := GetStore()
	if err != nil {
		t.Fatalf("failed to get store: %v", err)
	}
	queryIDMap, err := GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("failed to get query ID map: %v", err)
	}

	subscribedName, ok := queryIDMap["8621797084555037"]
	if !ok {
		t.Fatalf("expected query ID 8621797084555037 to be in map")
	}
	if subscribedName != "NewsletterSubscribed" {
		t.Fatalf("expected NewsletterSubscribed, got %s", subscribedName)
	}

	wt, ok := store[subscribedName]
	if !ok {
		t.Fatalf("expected wire type for %s to be in store", subscribedName)
	}
	if wt == nil {
		t.Fatalf("wire type for %s is nil", subscribedName)
	}
}
