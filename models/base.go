package models

import "gorm.io/gorm/clause"

type Map = map[string]any

var LockingClause = clause.Locking{Strength: "UPDATE"}
