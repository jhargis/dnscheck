package main

import "testing"

func TestExistent(t *testing.T) {
	result, _, err := resolve(referenceServer, "example.com")

	if err != nil {
		t.Fatal("an error occured")
	}

	if len(result) != 1 {
		t.Fatal("invalid number of records returned:", len(result))
	}
}

func TestNotExistent(t *testing.T) {
	result, authenticated, err := resolve(referenceServer, "xxx.example.com")

	if err != nil {
		t.Fatal("an error occured")
	}

	if authenticated {
		t.Fatal("answer should not be authenticated")
	}

	if len(result) > 0 {
		t.Fatal("no records expected")
	}
}

func TestAuthenticated(t *testing.T) {
	result, authenticated, err := resolve(referenceServer, "www.dnssec-tools.org")

	if err != nil {
		t.Fatal("an error occured")
	}

	if !authenticated {
		t.Fatal("answer not authenticated")
	}

	if len(result) != 1 {
		t.Fatal("invalid number of records returned:", len(result))
	}
}

func TestUnreachable(t *testing.T) {
	_, _, err := resolve("127.1.2.3", "example.com")

	if err == nil {
		t.Fatal("no error returned")
	}
	if err.Error() != "connection refused" {
		t.Fatal("unexpected error", err)
	}
}

func TestPtrName(t *testing.T) {
	result := ptrName("8.8.8.8")

	if result != "google-public-dns-a.google.com." {
		t.Fatal("invalid result:", result)
	}
}

func TestVersion(t *testing.T) {
	result := version("82.96.65.2")

	if result != "Make my day" {
		t.Fatal("invalid result:", result)
	}
}
