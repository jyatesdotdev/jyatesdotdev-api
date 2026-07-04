package visits

import (
	"context"
	"sort"
)

type VisitStats struct {
	Total     int64           `json:"total"`
	Countries []CountryVisits `json:"countries"`
}

type Service interface {
	RecordVisit(ctx context.Context, country, countryName, ipAddress string) error
	GetStats(ctx context.Context) (VisitStats, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) RecordVisit(ctx context.Context, country, countryName, ipAddress string) error {
	return s.repo.IncrementCountryVisit(ctx, country, countryName, ipAddress)
}

func (s *service) GetStats(ctx context.Context) (VisitStats, error) {
	countries, err := s.repo.GetCountryVisits(ctx)
	if err != nil {
		return VisitStats{}, err
	}

	sort.Slice(countries, func(i, j int) bool {
		if countries[i].Count != countries[j].Count {
			return countries[i].Count > countries[j].Count
		}
		return countries[i].Country < countries[j].Country
	})

	var total int64
	for _, c := range countries {
		total += c.Count
	}

	return VisitStats{Total: total, Countries: countries}, nil
}
