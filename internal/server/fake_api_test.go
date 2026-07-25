package server

import (
	"context"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// fakeAPI implements promv1.API for tests. Only the methods exercised by the
// server tests carry behaviour; the rest return zero values.
type fakeAPI struct {
	// Err, when set, is returned by every method that has no dedicated hook.
	Err              error
	QueryFn          func(ctx context.Context, query string, ts time.Time) (model.Value, promv1.Warnings, error)
	QueryRangeFn     func(ctx context.Context, query string, r promv1.Range) (model.Value, promv1.Warnings, error)
	SeriesFn         func(ctx context.Context, matches []string) ([]model.LabelSet, promv1.Warnings, error)
	LabelNamesFn     func(ctx context.Context) ([]string, promv1.Warnings, error)
	LabelValuesFn    func(ctx context.Context, label string) (model.LabelValues, promv1.Warnings, error)
	MetadataFn       func(ctx context.Context, metric, limit string) (map[string][]promv1.Metadata, error)
	TSDBFn           func(ctx context.Context) (promv1.TSDBResult, error)
	RulesFn          func(ctx context.Context) (promv1.RulesResult, error)
	TargetsFn        func(ctx context.Context) (promv1.TargetsResult, error)
	QueryExemplarsFn func(ctx context.Context, query string) ([]promv1.ExemplarQueryResult, error)
}

func (f *fakeAPI) Alerts(context.Context) (promv1.AlertsResult, error) {
	return promv1.AlertsResult{}, f.Err
}
func (f *fakeAPI) AlertManagers(context.Context) (promv1.AlertManagersResult, error) {
	return promv1.AlertManagersResult{}, f.Err
}
func (f *fakeAPI) CleanTombstones(context.Context) error { return nil }
func (f *fakeAPI) Config(context.Context) (promv1.ConfigResult, error) {
	return promv1.ConfigResult{}, f.Err
}
func (f *fakeAPI) DeleteSeries(context.Context, []string, time.Time, time.Time) error {
	return nil
}
func (f *fakeAPI) Flags(context.Context) (promv1.FlagsResult, error) {
	return promv1.FlagsResult{}, f.Err
}

func (f *fakeAPI) LabelNames(ctx context.Context, _ []string, _, _ time.Time, _ ...promv1.Option) ([]string, promv1.Warnings, error) {
	if f.LabelNamesFn != nil {
		return f.LabelNamesFn(ctx)
	}
	return nil, nil, nil
}

func (f *fakeAPI) LabelValues(ctx context.Context, label string, _ []string, _, _ time.Time, _ ...promv1.Option) (model.LabelValues, promv1.Warnings, error) {
	if f.LabelValuesFn != nil {
		return f.LabelValuesFn(ctx, label)
	}
	return nil, nil, nil
}

func (f *fakeAPI) Query(ctx context.Context, query string, ts time.Time, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if f.QueryFn != nil {
		return f.QueryFn(ctx, query, ts)
	}
	return model.Vector{}, nil, nil
}

func (f *fakeAPI) QueryRange(ctx context.Context, query string, r promv1.Range, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if f.QueryRangeFn != nil {
		return f.QueryRangeFn(ctx, query, r)
	}
	return model.Matrix{}, nil, nil
}

func (f *fakeAPI) QueryExemplars(ctx context.Context, query string, _, _ time.Time) ([]promv1.ExemplarQueryResult, error) {
	if f.QueryExemplarsFn != nil {
		return f.QueryExemplarsFn(ctx, query)
	}
	return nil, nil
}

func (f *fakeAPI) Buildinfo(context.Context) (promv1.BuildinfoResult, error) {
	return promv1.BuildinfoResult{}, f.Err
}
func (f *fakeAPI) Runtimeinfo(context.Context) (promv1.RuntimeinfoResult, error) {
	return promv1.RuntimeinfoResult{}, f.Err
}

func (f *fakeAPI) Series(ctx context.Context, matches []string, _, _ time.Time, _ ...promv1.Option) ([]model.LabelSet, promv1.Warnings, error) {
	if f.SeriesFn != nil {
		return f.SeriesFn(ctx, matches)
	}
	return nil, nil, nil
}

func (f *fakeAPI) Snapshot(context.Context, bool) (promv1.SnapshotResult, error) {
	return promv1.SnapshotResult{}, nil
}

func (f *fakeAPI) Rules(ctx context.Context) (promv1.RulesResult, error) {
	if f.RulesFn != nil {
		return f.RulesFn(ctx)
	}
	return promv1.RulesResult{}, nil
}

func (f *fakeAPI) Targets(ctx context.Context) (promv1.TargetsResult, error) {
	if f.TargetsFn != nil {
		return f.TargetsFn(ctx)
	}
	return promv1.TargetsResult{}, nil
}

func (f *fakeAPI) TargetsMetadata(context.Context, string, string, string) ([]promv1.MetricMetadata, error) {
	return nil, nil
}

func (f *fakeAPI) Metadata(ctx context.Context, metric, limit string) (map[string][]promv1.Metadata, error) {
	if f.MetadataFn != nil {
		return f.MetadataFn(ctx, metric, limit)
	}
	return nil, nil
}

func (f *fakeAPI) TSDB(ctx context.Context, _ ...promv1.Option) (promv1.TSDBResult, error) {
	if f.TSDBFn != nil {
		return f.TSDBFn(ctx)
	}
	return promv1.TSDBResult{}, f.Err
}

func (f *fakeAPI) WalReplay(context.Context) (promv1.WalReplayStatus, error) {
	return promv1.WalReplayStatus{}, f.Err
}
