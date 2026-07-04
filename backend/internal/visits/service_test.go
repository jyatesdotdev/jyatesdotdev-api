package visits

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) IncrementCountryVisit(ctx context.Context, country, countryName, ipAddress string) error {
	args := m.Called(ctx, country, countryName, ipAddress)
	return args.Error(0)
}

func (m *MockRepository) GetCountryVisits(ctx context.Context) ([]CountryVisits, error) {
	args := m.Called(ctx)
	return args.Get(0).([]CountryVisits), args.Error(1)
}

func TestGetStats_SortsAndTotals(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewService(mockRepo)

	mockRepo.On("GetCountryVisits", mock.Anything).Return([]CountryVisits{
		{Country: "DE", CountryName: "Germany", Count: 2},
		{Country: "US", CountryName: "United States", Count: 10},
		{Country: "CA", CountryName: "Canada", Count: 2},
	}, nil)

	stats, err := svc.GetStats(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, int64(14), stats.Total)
	// Sorted by count desc, then country code for ties
	assert.Equal(t, "US", stats.Countries[0].Country)
	assert.Equal(t, "CA", stats.Countries[1].Country)
	assert.Equal(t, "DE", stats.Countries[2].Country)
}

func TestRecordVisit_PassesThrough(t *testing.T) {
	mockRepo := new(MockRepository)
	svc := NewService(mockRepo)

	mockRepo.On("IncrementCountryVisit", mock.Anything, "US", "United States", "1.2.3.4").Return(nil)

	err := svc.RecordVisit(context.Background(), "US", "United States", "1.2.3.4")

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}
