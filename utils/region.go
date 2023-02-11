package utils

import (
	"MOSS_backend/data"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"strings"
)

var searcher *xdb.Searcher

func init() {
	var err error
	searcher, err = xdb.NewWithBuffer(data.Ip2RegionDBFile)
	if err != nil {
		panic(err)
	}
}

func IsInChina(ip string) (bool, error) {
	region, err := searcher.SearchByStr(ip)
	if err != nil {
		return false, err
	}
	regionTable := strings.Split(region, "|")
	return regionTable[0] == "中国", nil
}
