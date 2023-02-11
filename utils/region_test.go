package utils

import (
	"fmt"
	"testing"
)

func TestRegion(t *testing.T) {
	const ip = "4.4.4.4"
	region, err := searcher.SearchByStr(ip)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(region)

	fmt.Println(IsInChina("4.4.4.4"))
}
