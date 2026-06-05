package ui

import (
	"context"

	"github.com/vasylenko/cellarman/internal/brew"
)

// Brew is the slice of the brew client the UI depends on. Declaring it on the
// consumer side (rather than exporting an interface from the brew package) keeps
// the contract minimal and lets tests substitute a fake without a real brew.
// *brew.Client satisfies it.
type Brew interface {
	Installed(ctx context.Context) ([]brew.Formula, []brew.Cask, error)
	Info(ctx context.Context, name string, kind brew.Kind) (*brew.Formula, *brew.Cask, error)
	Taps(ctx context.Context) ([]brew.Tap, error)
	Outdated(ctx context.Context) (*brew.OutdatedReport, error)
	Search(ctx context.Context, term string, kind brew.Kind, evalAll bool) ([]string, error)
	Descriptions(ctx context.Context, names []string, kind brew.Kind) (map[string]string, error)
	Doctor(ctx context.Context) (*brew.DoctorReport, error)
	Install(ctx context.Context, kind brew.Kind, names ...string) (<-chan brew.Event, error)
	Upgrade(ctx context.Context, names ...string) (<-chan brew.Event, error)
	Cleanup(ctx context.Context) (<-chan brew.Event, error)
	Autoremove(ctx context.Context) (<-chan brew.Event, error)
}
