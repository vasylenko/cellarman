package ui

import (
	"context"

	"github.com/serhii-vasylenko/brew-tui/internal/brew"
)

// fakeBrew is an in-memory Brew for view tests: load Cmds resolve synchronously
// and deterministically, with no real brew, exec, or network.
type fakeBrew struct {
	formulae    []brew.Formula
	casks       []brew.Cask
	taps        []brew.Tap
	outdated    *brew.OutdatedReport
	doctor      *brew.DoctorReport
	searchHits  []string
	descs       map[string]string
	infoFormula *brew.Formula
	infoCask    *brew.Cask
	err         error
	events      []brew.Event    // streamed lines for Upgrade/Cleanup/Autoremove
	streamErr   error           // when set, stream starts fail (os.Pipe/cmd.Start equivalent)
	upgradeCtx  context.Context // captured so abort tests can observe cancellation
}

func (f *fakeBrew) Installed(context.Context) ([]brew.Formula, []brew.Cask, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.formulae, f.casks, nil
}

func (f *fakeBrew) Info(_ context.Context, _ string, kind brew.Kind) (*brew.Formula, *brew.Cask, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	if kind == brew.KindCask {
		return nil, f.infoCask, nil
	}
	return f.infoFormula, nil, nil
}

func (f *fakeBrew) Descriptions(_ context.Context, _ []string, _ brew.Kind) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.descs, nil
}

func (f *fakeBrew) Taps(context.Context) ([]brew.Tap, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.taps, nil
}

func (f *fakeBrew) Outdated(context.Context) (*brew.OutdatedReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.outdated, nil
}

func (f *fakeBrew) Search(context.Context, string, brew.Kind, bool) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.searchHits, nil
}

func (f *fakeBrew) Doctor(context.Context) (*brew.DoctorReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.doctor, nil
}

func (f *fakeBrew) Upgrade(ctx context.Context, _ ...string) (<-chan brew.Event, error) {
	f.upgradeCtx = ctx
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return f.stream(), nil
}
func (f *fakeBrew) Cleanup(context.Context) (<-chan brew.Event, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return f.stream(), nil
}
func (f *fakeBrew) Autoremove(context.Context) (<-chan brew.Event, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return f.stream(), nil
}

// stream replays canned line events followed by a terminal Done on a buffered,
// already-closed channel — consumers drain it without a live goroutine.
func (f *fakeBrew) stream() <-chan brew.Event {
	ch := make(chan brew.Event, len(f.events)+1)
	for _, e := range f.events {
		ch <- e
	}
	ch <- brew.Event{Done: true}
	close(ch)
	return ch
}

func sampleBrew() *fakeBrew {
	return &fakeBrew{
		formulae: []brew.Formula{
			{
				Name: "go", Tap: "homebrew/core", Desc: "Go programming language",
				License: "BSD-3-Clause", Homepage: "https://go.dev",
				Versions:  brew.Versions{Stable: "1.26.3"},
				Installed: []brew.InstalledKeg{{Version: "1.26.3"}},
			},
			{
				Name: "gnutls", Tap: "homebrew/core", Desc: "TLS library",
				Versions:     brew.Versions{Stable: "3.8.13_2"},
				Installed:    []brew.InstalledKeg{{Version: "3.8.13_1"}},
				Outdated:     true,
				Dependencies: []string{"gmp", "nettle"},
			},
		},
		casks: []brew.Cask{
			{Token: "qlmarkdown", Names: []string{"sbarex QLMarkdown"}, Version: "1.5.1", Installed: "1.5.1", Tap: "homebrew/cask"},
		},
		taps: []brew.Tap{
			{Name: "homebrew/core", Official: true, FormulaNames: []string{"go", "gnutls"}},
			{Name: "hashicorp/tap", FormulaNames: []string{"terraform"}},
		},
	}
}
