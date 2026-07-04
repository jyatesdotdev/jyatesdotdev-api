package visits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockService struct {
	mock.Mock
}

func (m *MockService) RecordVisit(ctx context.Context, country, countryName, ipAddress string) error {
	args := m.Called(ctx, country, countryName, ipAddress)
	return args.Error(0)
}

func (m *MockService) GetStats(ctx context.Context) (VisitStats, error) {
	args := m.Called(ctx)
	return args.Get(0).(VisitStats), args.Error(1)
}

func TestWhereAmI(t *testing.T) {
	handler := NewHandler(new(MockService))

	req := httptest.NewRequest("GET", "/api/v1/geo", nil)
	req.Header.Set("CloudFront-Viewer-Country", "US")
	req.Header.Set("CloudFront-Viewer-Country-Name", "United States")
	req.Header.Set("CloudFront-Viewer-City", "Seattle")
	req.Header.Set("CloudFront-Viewer-Time-Zone", "America/Los_Angeles")
	w := httptest.NewRecorder()

	handler.WhereAmI(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp GeoResponse
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "US", resp.Country)
	assert.Equal(t, "United States", resp.CountryName)
	assert.Equal(t, "Seattle", resp.City)
	assert.Equal(t, "America/Los_Angeles", resp.TimeZone)
}

func TestWhereAmI_NoHeaders(t *testing.T) {
	handler := NewHandler(new(MockService))

	req := httptest.NewRequest("GET", "/api/v1/geo", nil)
	w := httptest.NewRecorder()

	handler.WhereAmI(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp GeoResponse
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Empty(t, resp.Country)
}

func TestRecordVisit(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("RecordVisit", mock.Anything, "DE", "Germany", "1.2.3.4").Return(nil)

	req := httptest.NewRequest("POST", "/api/v1/visits", nil)
	req.Header.Set("CloudFront-Viewer-Country", "DE")
	req.Header.Set("CloudFront-Viewer-Country-Name", "Germany")
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	w := httptest.NewRecorder()

	handler.RecordVisit(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestRecordVisit_InvalidCountryIsNoOp(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	for _, country := range []string{"", "usa", "U1", "<script>"} {
		req := httptest.NewRequest("POST", "/api/v1/visits", nil)
		if country != "" {
			req.Header.Set("CloudFront-Viewer-Country", country)
		}
		w := httptest.NewRecorder()

		handler.RecordVisit(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	}
	mockSvc.AssertNotCalled(t, "RecordVisit", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRecordVisit_RateLimited(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("RecordVisit", mock.Anything, "US", "", mock.Anything).Return(ErrRateLimited)

	req := httptest.NewRequest("POST", "/api/v1/visits", nil)
	req.Header.Set("CloudFront-Viewer-Country", "US")
	w := httptest.NewRecorder()

	handler.RecordVisit(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestGetStats(t *testing.T) {
	mockSvc := new(MockService)
	handler := NewHandler(mockSvc)

	mockSvc.On("GetStats", mock.Anything).Return(VisitStats{
		Total: 12,
		Countries: []CountryVisits{
			{Country: "US", CountryName: "United States", Count: 10},
			{Country: "DE", CountryName: "Germany", Count: 2},
		},
	}, nil)

	req := httptest.NewRequest("GET", "/api/v1/visits", nil)
	req.Header.Set("CloudFront-Viewer-Country", "DE")
	w := httptest.NewRecorder()

	handler.GetStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp StatsResponse
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, int64(12), resp.Total)
	assert.Len(t, resp.Countries, 2)
	assert.Equal(t, "US", resp.Countries[0].Country)
	assert.Equal(t, "DE", resp.You)
}
