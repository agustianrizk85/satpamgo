package listquery

import (
	"net/http"
	"strconv"
	"strings"
)

type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

type Options struct {
	AllowedSortBy    []string
	DefaultSortBy    string
	DefaultSortOrder SortOrder
	DefaultPageSize  int
	MaxPageSize      int
}

type Query struct {
	Page      int       `json:"page"`
	PageSize  int       `json:"pageSize"`
	Offset    int       `json:"-"`
	SortBy    string    `json:"sortBy"`
	SortOrder SortOrder `json:"sortOrder"`
}

type Response[T any] struct {
	Data       []T `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		TotalData  int `json:"totalData"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
	Sort struct {
		SortBy    string    `json:"sortBy"`
		SortOrder SortOrder `json:"sortOrder"`
	} `json:"sort"`
}

func Parse(r *http.Request, options Options) (Query, string, bool) {
	defaultPageSize := options.DefaultPageSize
	if defaultPageSize == 0 {
		defaultPageSize = 20
	}

	maxPageSize := options.MaxPageSize
	if maxPageSize == 0 {
		maxPageSize = 100
	}

	defaultSortOrder := options.DefaultSortOrder
	if defaultSortOrder == "" {
		defaultSortOrder = SortDesc
	}

	page, ok := parsePositiveInt(r.URL.Query().Get("page"), 1)
	if !ok {
		return Query{}, "page must be integer >= 1", false
	}

	pageSize, ok := parsePositiveInt(r.URL.Query().Get("pageSize"), defaultPageSize)
	if !ok {
		return Query{}, "pageSize must be integer >= 1", false
	}
	if pageSize > maxPageSize {
		return Query{}, "pageSize cannot be more than " + strconv.Itoa(maxPageSize), false
	}

	sortBy := strings.TrimSpace(r.URL.Query().Get("sortBy"))
	if sortBy == "" {
		sortBy = options.DefaultSortBy
	}
	if !contains(options.AllowedSortBy, sortBy) {
		return Query{}, "sortBy invalid. Allowed: " + strings.Join(options.AllowedSortBy, ", "), false
	}

	sortOrder := defaultSortOrder
	rawSortOrder := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sortOrder")))
	if rawSortOrder != "" {
		if rawSortOrder != string(SortAsc) && rawSortOrder != string(SortDesc) {
			return Query{}, "sortOrder must be asc or desc", false
		}
		sortOrder = SortOrder(rawSortOrder)
	}

	return Query{
		Page:      page,
		PageSize:  pageSize,
		Offset:    (page - 1) * pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}, "", true
}

func BuildResponse[T any](data []T, query Query, totalData int) Response[T] {
	var out Response[T]
	out.Data = data
	out.Pagination.Page = query.Page
	out.Pagination.PageSize = query.PageSize
	out.Pagination.TotalData = totalData
	if totalData > 0 {
		out.Pagination.TotalPages = (totalData + query.PageSize - 1) / query.PageSize
	}
	out.Sort.SortBy = query.SortBy
	out.Sort.SortOrder = query.SortOrder
	return out
}

func parsePositiveInt(raw string, fallback int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, false
	}

	return value, true
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
