package common

func CalculatePagination(page, limit int, totalItems int64) Pagination {
	if limit <= 0 {
		limit = 10
	}
	if page <= 0 {
		page = 1
	}
	totalPages := (totalItems + int64(limit) - 1) / int64(limit)
	return Pagination{
		Page:       page,
		Limit:      limit,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}
