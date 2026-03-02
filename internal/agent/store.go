package agent

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

const (
	// rawBufferDuration is how long we keep raw samples before aggregating.
	rawBufferDuration = 5 * time.Minute

	// fineGranularity is the bucket size for recent data (<24h).
	fineGranularity = 5 * time.Minute

	// coarseGranularity is the bucket size for older data (24h–7d).
	coarseGranularity = 1 * time.Hour

	// fineRetention is how long we keep fine-grained buckets.
	fineRetention = 24 * time.Hour

	// coarseRetention is how long we keep coarse-grained buckets.
	coarseRetention = 7 * 24 * time.Hour
)

// rawSample is a single metric observation from one scrape.
type rawSample struct {
	timestamp        time.Time
	cpuNanoCores     uint64
	memUsageBytes    uint64
	memWorkingSet    uint64
}

// bucket holds pre-aggregated metrics over a time window.
type bucket struct {
	start       time.Time
	end         time.Time
	granularity time.Duration
	sampleCount int

	cpuNanoCores    models.MetricAggregate
	memUsageBytes   models.MetricAggregate
	memWorkingSet   models.MetricAggregate
}

// containerTimeSeries holds all metric data for one container.
type containerTimeSeries struct {
	podName       string
	podNamespace  string
	containerName string

	// Latest metadata from the most recent sample.
	cpuRequestMillis  int64
	cpuLimitMillis    int64
	memRequestBytes   int64
	memLimitBytes     int64
	restartCount      int32
	qosClass          string
	ownerKind         string
	ownerName         string

	// Raw samples waiting to be aggregated.
	rawSamples []rawSample

	// Fine-grained buckets (5 min), kept for 24h.
	fineBuckets []bucket

	// Coarse-grained buckets (1 hr), kept for 7d.
	coarseBuckets []bucket
}

// Store is a thread-safe rolling window in-memory metric store with downsampling.
type Store struct {
	mu         sync.RWMutex
	containers map[string]*containerTimeSeries // keyed by namespace/pod/container
	now        func() time.Time               // injectable clock for testing
}

// NewStore creates a new Store.
func NewStore() *Store {
	return &Store{
		containers: make(map[string]*containerTimeSeries),
		now:        time.Now,
	}
}

// Record stores a raw metric sample for the given container.
func (s *Store) Record(m models.ContainerMetrics) {
	key := models.ContainerKey(m.PodNamespace, m.PodName, m.ContainerName)

	s.mu.Lock()
	defer s.mu.Unlock()

	ts, exists := s.containers[key]
	if !exists {
		ts = &containerTimeSeries{
			podName:       m.PodName,
			podNamespace:  m.PodNamespace,
			containerName: m.ContainerName,
		}
		s.containers[key] = ts
	}

	// Update metadata from latest sample.
	ts.cpuRequestMillis = m.CPURequestMillis
	ts.cpuLimitMillis = m.CPULimitMillis
	ts.memRequestBytes = m.MemoryRequestBytes
	ts.memLimitBytes = m.MemoryLimitBytes
	ts.restartCount = m.RestartCount
	ts.qosClass = m.QoSClass
	if m.OwnerKind != "" {
		ts.ownerKind = m.OwnerKind
		ts.ownerName = m.OwnerName
	}

	ts.rawSamples = append(ts.rawSamples, rawSample{
		timestamp:     m.Timestamp,
		cpuNanoCores:  m.CPUUsageNanoCores,
		memUsageBytes: m.MemoryUsageBytes,
		memWorkingSet: m.MemoryWorkingSetBytes,
	})

	// Aggregate raw samples into fine buckets if enough time has passed.
	s.maybeAggregate(ts)

	// Downsample fine buckets into coarse buckets and evict expired data.
	s.maybeDownsample(ts)
}

// maybeAggregate converts raw samples older than rawBufferDuration into fine buckets.
func (s *Store) maybeAggregate(ts *containerTimeSeries) {
	now := s.now()
	cutoff := now.Add(-rawBufferDuration)

	// Find samples that are old enough to aggregate.
	var toAggregate []rawSample
	var remaining []rawSample

	for _, sample := range ts.rawSamples {
		if sample.timestamp.Before(cutoff) {
			toAggregate = append(toAggregate, sample)
		} else {
			remaining = append(remaining, sample)
		}
	}

	if len(toAggregate) == 0 {
		return
	}

	ts.rawSamples = remaining

	// Group samples by fine-granularity bucket.
	bucketGroups := make(map[time.Time][]rawSample)
	for _, sample := range toAggregate {
		bucketStart := sample.timestamp.Truncate(fineGranularity)
		bucketGroups[bucketStart] = append(bucketGroups[bucketStart], sample)
	}

	for start, samples := range bucketGroups {
		b := aggregateSamples(samples, start, fineGranularity)
		ts.fineBuckets = appendBucket(ts.fineBuckets, b)
	}
}

// maybeDownsample converts fine buckets older than fineRetention into coarse buckets
// and evicts expired coarse buckets.
func (s *Store) maybeDownsample(ts *containerTimeSeries) {
	now := s.now()
	fineCutoff := now.Add(-fineRetention)
	coarseCutoff := now.Add(-coarseRetention)

	// Find fine buckets that need to become coarse.
	var toDownsample []bucket
	var remainingFine []bucket

	for _, b := range ts.fineBuckets {
		if b.start.Before(fineCutoff) {
			toDownsample = append(toDownsample, b)
		} else {
			remainingFine = append(remainingFine, b)
		}
	}

	ts.fineBuckets = remainingFine

	if len(toDownsample) > 0 {
		// Group fine buckets by coarse bucket boundary.
		coarseGroups := make(map[time.Time][]bucket)
		for _, b := range toDownsample {
			coarseStart := b.start.Truncate(coarseGranularity)
			coarseGroups[coarseStart] = append(coarseGroups[coarseStart], b)
		}

		for start, fineBuckets := range coarseGroups {
			merged := mergeBuckets(fineBuckets, start, coarseGranularity)
			ts.coarseBuckets = appendBucket(ts.coarseBuckets, merged)
		}
	}

	// Evict expired coarse buckets.
	var remainingCoarse []bucket
	for _, b := range ts.coarseBuckets {
		if !b.start.Before(coarseCutoff) {
			remainingCoarse = append(remainingCoarse, b)
		}
	}
	ts.coarseBuckets = remainingCoarse
}

// appendBucket inserts a bucket into a sorted slice, merging if one already exists
// for the same time window.
func appendBucket(buckets []bucket, b bucket) []bucket {
	for i, existing := range buckets {
		if existing.start.Equal(b.start) && existing.granularity == b.granularity {
			// Merge into existing bucket.
			buckets[i] = mergeTwo(existing, b)
			return buckets
		}
	}
	buckets = append(buckets, b)
	// Keep sorted by start time.
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].start.Before(buckets[j].start)
	})
	return buckets
}

// aggregateSamples computes P50/P95/P99/Max over a set of raw samples.
func aggregateSamples(samples []rawSample, start time.Time, granularity time.Duration) bucket {
	cpuValues := make([]float64, len(samples))
	memUsageValues := make([]float64, len(samples))
	memWSValues := make([]float64, len(samples))

	for i, s := range samples {
		cpuValues[i] = float64(s.cpuNanoCores)
		memUsageValues[i] = float64(s.memUsageBytes)
		memWSValues[i] = float64(s.memWorkingSet)
	}

	return bucket{
		start:         start,
		end:           start.Add(granularity),
		granularity:   granularity,
		sampleCount:   len(samples),
		cpuNanoCores:  computeAggregate(cpuValues),
		memUsageBytes: computeAggregate(memUsageValues),
		memWorkingSet: computeAggregate(memWSValues),
	}
}

// mergeBuckets merges multiple fine-grained buckets into a single coarser bucket.
func mergeBuckets(buckets []bucket, start time.Time, granularity time.Duration) bucket {
	if len(buckets) == 1 {
		b := buckets[0]
		b.start = start
		b.end = start.Add(granularity)
		b.granularity = granularity
		return b
	}

	result := buckets[0]
	for _, b := range buckets[1:] {
		result = mergeTwo(result, b)
	}
	result.start = start
	result.end = start.Add(granularity)
	result.granularity = granularity
	return result
}

// mergeTwo merges two buckets by taking the weighted average of percentiles
// and max of maxima.
func mergeTwo(a, b bucket) bucket {
	totalSamples := a.sampleCount + b.sampleCount
	return bucket{
		start:       a.start,
		end:         a.end,
		granularity: a.granularity,
		sampleCount: totalSamples,
		cpuNanoCores:  mergeAggregates(a.cpuNanoCores, a.sampleCount, b.cpuNanoCores, b.sampleCount),
		memUsageBytes: mergeAggregates(a.memUsageBytes, a.sampleCount, b.memUsageBytes, b.sampleCount),
		memWorkingSet: mergeAggregates(a.memWorkingSet, a.sampleCount, b.memWorkingSet, b.sampleCount),
	}
}

// mergeAggregates merges two MetricAggregates using weighted averages for
// percentiles and max for the maximum value.
func mergeAggregates(a models.MetricAggregate, aCount int, b models.MetricAggregate, bCount int) models.MetricAggregate {
	total := float64(aCount + bCount)
	if total == 0 {
		return models.MetricAggregate{}
	}
	wa := float64(aCount) / total
	wb := float64(bCount) / total

	return models.MetricAggregate{
		P50: a.P50*wa + b.P50*wb,
		P95: math.Max(a.P95, b.P95), // Conservative: take the higher P95
		P99: math.Max(a.P99, b.P99), // Conservative: take the higher P99
		Max: math.Max(a.Max, b.Max),
	}
}

// computeAggregate calculates P50, P95, P99, and Max from a slice of values.
func computeAggregate(values []float64) models.MetricAggregate {
	if len(values) == 0 {
		return models.MetricAggregate{}
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return models.MetricAggregate{
		P50: percentile(sorted, 0.50),
		P95: percentile(sorted, 0.95),
		P99: percentile(sorted, 0.99),
		Max: sorted[len(sorted)-1],
	}
}

// percentile returns the p-th percentile from a sorted slice (nearest-rank method).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// GetAggregates returns all aggregated metrics for every tracked container.
// Results are keyed by container key (namespace/pod/container).
func (s *Store) GetAggregates() map[string][]models.AggregatedMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]models.AggregatedMetrics)

	for key, ts := range s.containers {
		var aggs []models.AggregatedMetrics

		for _, b := range ts.coarseBuckets {
			aggs = append(aggs, bucketToAggregated(ts, b))
		}
		for _, b := range ts.fineBuckets {
			aggs = append(aggs, bucketToAggregated(ts, b))
		}

		// Include a live aggregate from raw samples if any exist.
		if len(ts.rawSamples) > 0 {
			now := s.now()
			liveBucket := aggregateSamples(ts.rawSamples, now.Add(-rawBufferDuration), rawBufferDuration)
			aggs = append(aggs, bucketToAggregated(ts, liveBucket))
		}

		if len(aggs) > 0 {
			result[key] = aggs
		}
	}

	return result
}

// mergedSummary computes a single merged bucket over all data for a container.
// Caller must hold at least a read lock.
func (ts *containerTimeSeries) mergedSummary(now time.Time) (bucket, bool) {
	var allBuckets []bucket
	allBuckets = append(allBuckets, ts.coarseBuckets...)
	allBuckets = append(allBuckets, ts.fineBuckets...)

	if len(ts.rawSamples) > 0 {
		allBuckets = append(allBuckets, aggregateSamples(ts.rawSamples, now.Add(-rawBufferDuration), rawBufferDuration))
	}

	if len(allBuckets) == 0 {
		return bucket{}, false
	}

	merged := allBuckets[0]
	for _, b := range allBuckets[1:] {
		merged = mergeTwo(merged, b)
	}
	return merged, true
}

// meta returns a ContainerMeta snapshot. Caller must hold at least a read lock.
func (ts *containerTimeSeries) meta() ContainerMeta {
	return ContainerMeta{
		PodName:          ts.podName,
		PodNamespace:     ts.podNamespace,
		ContainerName:    ts.containerName,
		CPURequestMillis: ts.cpuRequestMillis,
		CPULimitMillis:   ts.cpuLimitMillis,
		MemRequestBytes:  ts.memRequestBytes,
		MemLimitBytes:    ts.memLimitBytes,
		RestartCount:     ts.restartCount,
		QoSClass:         ts.qosClass,
		OwnerKind:        ts.ownerKind,
		OwnerName:        ts.ownerName,
	}
}

// dataWindow computes the time span of available data. Caller must hold at least a read lock.
func (ts *containerTimeSeries) dataWindow() time.Duration {
	var earliest, latest time.Time

	if len(ts.coarseBuckets) > 0 {
		earliest = ts.coarseBuckets[0].start
		latest = ts.coarseBuckets[len(ts.coarseBuckets)-1].end
	}

	if len(ts.fineBuckets) > 0 {
		if earliest.IsZero() || ts.fineBuckets[0].start.Before(earliest) {
			earliest = ts.fineBuckets[0].start
		}
		if ts.fineBuckets[len(ts.fineBuckets)-1].end.After(latest) {
			latest = ts.fineBuckets[len(ts.fineBuckets)-1].end
		}
	}

	if len(ts.rawSamples) > 0 {
		if earliest.IsZero() || ts.rawSamples[0].timestamp.Before(earliest) {
			earliest = ts.rawSamples[0].timestamp
		}
		last := ts.rawSamples[len(ts.rawSamples)-1].timestamp
		if last.After(latest) {
			latest = last
		}
	}

	if earliest.IsZero() {
		return 0
	}
	return latest.Sub(earliest)
}

// GetContainerSummary returns a single aggregate over all available data for a container.
func (s *Store) GetContainerSummary(key string) (models.AggregatedMetrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, exists := s.containers[key]
	if !exists {
		return models.AggregatedMetrics{}, false
	}

	merged, ok := ts.mergedSummary(s.now())
	if !ok {
		return models.AggregatedMetrics{}, false
	}
	return bucketToAggregated(ts, merged), true
}

// GetContainerMeta returns metadata for a container (requests, limits, etc).
func (s *Store) GetContainerMeta(key string) (ContainerMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, exists := s.containers[key]
	if !exists {
		return ContainerMeta{}, false
	}
	return ts.meta(), true
}

// ContainerMeta holds non-metric metadata about a container.
type ContainerMeta struct {
	PodName          string
	PodNamespace     string
	ContainerName    string
	CPURequestMillis int64
	CPULimitMillis   int64
	MemRequestBytes  int64
	MemLimitBytes    int64
	RestartCount     int32
	QoSClass         string
	OwnerKind        string
	OwnerName        string
}

// ContainerKeys returns all tracked container keys.
func (s *Store) ContainerKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.containers))
	for k := range s.containers {
		keys = append(keys, k)
	}
	return keys
}

// DataWindow returns the total time duration of available data for a container.
func (s *Store) DataWindow(key string) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, exists := s.containers[key]
	if !exists {
		return 0
	}
	return ts.dataWindow()
}

// ContainerSnapshot holds a consistent point-in-time snapshot of a container's
// summary, metadata, and data window, captured under a single lock acquisition.
type ContainerSnapshot struct {
	Summary    models.AggregatedMetrics
	Meta       ContainerMeta
	DataWindow time.Duration
	OK         bool
}

// GetContainerSnapshot returns summary, meta, and data window for a container
// in a single lock acquisition, ensuring consistency across all three values.
func (s *Store) GetContainerSnapshot(key string) ContainerSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ts, exists := s.containers[key]
	if !exists {
		return ContainerSnapshot{}
	}

	merged, ok := ts.mergedSummary(s.now())
	if !ok {
		return ContainerSnapshot{}
	}

	return ContainerSnapshot{
		Summary:    bucketToAggregated(ts, merged),
		Meta:       ts.meta(),
		DataWindow: ts.dataWindow(),
		OK:         true,
	}
}

func bucketToAggregated(ts *containerTimeSeries, b bucket) models.AggregatedMetrics {
	return models.AggregatedMetrics{
		PodName:          ts.podName,
		PodNamespace:     ts.podNamespace,
		ContainerName:    ts.containerName,
		BucketStart:      b.start,
		BucketEnd:        b.end,
		SampleCount:      b.sampleCount,
		CPUNanoCores:     b.cpuNanoCores,
		MemoryUsageBytes: b.memUsageBytes,
		MemoryWorkingSet: b.memWorkingSet,
	}
}
