package store

import "testing"

func TestNpub(t *testing.T) {
	// NIP-19's example key
	got := npub("3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d")
	if want := "npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6"; got != want {
		t.Fatalf("npub = %q, want %q", got, want)
	}
	if npub("") != "" || npub("zz") != "" {
		t.Fatal("malformed keys should give an empty npub")
	}
}
