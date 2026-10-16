package metrics

import "time"

type bucket struct {
	sum, min, max float64
	count         int64
}

func (b *bucket) add(v float64) {
	if b.count == 0 || v < b.min {
		b.min = v
	}
	if b.count == 0 || v > b.max {
		b.max = v
	}
	b.sum += v
	b.count++
}

type seriesKey struct {
	target string
	metric int
}

type openBucket struct {
	start int64
	b     bucket
}

// aggregator feeds every raw sample into the open bucket of each tier and hands back
// buckets once their time window has passed. Nothing is stored at raw resolution.
type aggregator struct {
	open map[seriesKey][]*openBucket
}

func newAggregator() *aggregator {
	return &aggregator{open: map[seriesKey][]*openBucket{}}
}

func (a *aggregator) add(ts time.Time, target, metric string, v float64) []point {
	def, ok := metricDefs[metric]
	if !ok {
		return nil
	}
	key := seriesKey{target: target, metric: def.id}
	buckets, ok := a.open[key]
	if !ok {
		buckets = make([]*openBucket, len(tiers))
		a.open[key] = buckets
	}
	var closed []point
	for i, t := range tiers {
		step := int64(t.step.Seconds())
		start := ts.Unix() / step * step
		ob := buckets[i]
		if ob != nil && ob.start != start {
			closed = append(closed, point{target: target, metric: def.id, tier: i, start: ob.start, b: ob.b})
			ob = nil
		}
		if ob == nil {
			ob = &openBucket{start: start}
			buckets[i] = ob
		}
		ob.b.add(v)
	}
	return closed
}

// flush closes every bucket whose window ended before now, including series that
// stopped receiving samples (e.g. an app that was stopped).
func (a *aggregator) flush(now time.Time) []point {
	var closed []point
	for key, buckets := range a.open {
		empty := true
		for i, ob := range buckets {
			if ob == nil {
				continue
			}
			if ob.start+int64(tiers[i].step.Seconds()) <= now.Unix() {
				closed = append(closed, point{target: key.target, metric: key.metric, tier: i, start: ob.start, b: ob.b})
				buckets[i] = nil
				continue
			}
			empty = false
		}
		if empty {
			delete(a.open, key)
		}
	}
	return closed
}
