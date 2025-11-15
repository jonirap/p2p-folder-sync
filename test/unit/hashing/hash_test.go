package hashing_test

import (
	"fmt"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

func TestHashStringBasic(t *testing.T) {
	fmt.Printf("empty: %s\n", hashing.HashString([]byte("")))
	fmt.Printf("hello world: %s\n", hashing.HashString([]byte("hello world")))
	fmt.Printf("a: %s\n", hashing.HashString([]byte("a")))
	fmt.Printf("abc: %s\n", hashing.HashString([]byte("abc")))
}
