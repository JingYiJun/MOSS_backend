package data

import _ "embed"

//go:embed ip2region.xdb
var Ip2RegionDBFile []byte

//go:embed meta.json
var MetaData []byte
