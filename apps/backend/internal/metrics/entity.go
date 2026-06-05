package metrics

// metricsSample is the GORM persistence struct for the metrics_samples
// table. Unexported — only this package constructs or reads it.
type metricsSample struct {
	ID          string  `gorm:"column:id;primaryKey"`
	CollectedAt string  `gorm:"column:collected_at;not null"` // RFC3339Nano UTC
	Metric      string  `gorm:"column:metric;not null"`
	Value       float64 `gorm:"column:value;not null"`
}

// TableName satisfies gorm.Tabler.
func (metricsSample) TableName() string { return "metrics_samples" }
