package ratelimit

import (
	//"fmt"
	"sync"
	"sync/atomic"
)

// ==========================================
// Metrics Collector
// ==========================================

type Metrics struct {
	TotalRequests   uint64
	AllowedRequests uint64
	DeniedRequests  uint64
	Errors          uint64
	
	// Per-category metrics
	CategoryMetrics map[string]*CategoryMetrics
	mu              sync.RWMutex
}

type CategoryMetrics struct {
	Allowed uint64
	Denied  uint64
}

var globalMetrics = &Metrics{
	CategoryMetrics: make(map[string]*CategoryMetrics),
}

// RecordAllow records an allowed request
func RecordAllow(category string) {
	atomic.AddUint64(&globalMetrics.TotalRequests, 1)
	atomic.AddUint64(&globalMetrics.AllowedRequests, 1)
	
	globalMetrics.mu.Lock()
	if _, exists := globalMetrics.CategoryMetrics[category]; !exists {
		globalMetrics.CategoryMetrics[category] = &CategoryMetrics{}
	}
	atomic.AddUint64(&globalMetrics.CategoryMetrics[category].Allowed, 1)
	globalMetrics.mu.Unlock()
}

// RecordDeny records a denied request
func RecordDeny(category string) {
	atomic.AddUint64(&globalMetrics.TotalRequests, 1)
	atomic.AddUint64(&globalMetrics.DeniedRequests, 1)
	
	globalMetrics.mu.Lock()
	if _, exists := globalMetrics.CategoryMetrics[category]; !exists {
		globalMetrics.CategoryMetrics[category] = &CategoryMetrics{}
	}
	atomic.AddUint64(&globalMetrics.CategoryMetrics[category].Denied, 1)
	globalMetrics.mu.Unlock()
}

// RecordError records an error
func RecordError() {
	atomic.AddUint64(&globalMetrics.Errors, 1)
}

// GetMetrics returns current metrics
func GetMetrics() *Metrics {
	return globalMetrics
}

// AllowRate calculates the percentage of allowed requests
func (m *Metrics) AllowRate() float64 {
	total := atomic.LoadUint64(&m.TotalRequests)
	if total == 0 {
		return 100.0
	}
	allowed := atomic.LoadUint64(&m.AllowedRequests)
	return float64(allowed) / float64(total) * 100.0
}

// GetCategoryStats returns stats for a specific category
func (m *Metrics) GetCategoryStats(category string) *CategoryMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if stats, exists := m.CategoryMetrics[category]; exists {
		return stats
	}
	return &CategoryMetrics{}
}

// Reset resets all metrics
func ResetMetrics() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	
	atomic.StoreUint64(&globalMetrics.TotalRequests, 0)
	atomic.StoreUint64(&globalMetrics.AllowedRequests, 0)
	atomic.StoreUint64(&globalMetrics.DeniedRequests, 0)
	atomic.StoreUint64(&globalMetrics.Errors, 0)
	
	globalMetrics.CategoryMetrics = make(map[string]*CategoryMetrics)
}