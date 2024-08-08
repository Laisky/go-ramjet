package jav

import (
	"github.com/Laisky/go-ramjet/internal/tasks/jav/http"
	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	grp := web.Server.Group("/jav")
	grp.GET("/search", http.Search)
}
