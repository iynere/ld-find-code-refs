package parse

import (
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/git-flag-parser/parse/internal/ld"
)

// Since our hunking algorithm uses some maps, resulting slice orders are not deterministic
// We use these sorters to make sure the results are always in a deterministic order.
type byPath []ld.ReferenceHunksRep
type byOffset []ld.HunkRep

func (r byPath) Len() int           { return len(r) }
func (r byPath) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byPath) Less(i, j int) bool { return r[i].Path < r[j].Path }

func (h byOffset) Len() int           { return len(h) }
func (h byOffset) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h byOffset) Less(i, j int) bool { return h[i].Offset < h[j].Offset }

func Test_generateReferencesFromGrep(t *testing.T) {
	tests := []struct {
		name       string
		flags      []string
		grepResult [][]string
		ctxLines   int
		want       []grepResultLine
		exclude    string
	}{
		{
			name:  "succeeds",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", ":", "12", "someFlag"},
			},
			ctxLines: 0,
			want: []grepResultLine{
				{Path: "flags.txt", LineNum: 12, LineText: "someFlag", FlagKeys: []string{"someFlag"}},
			},
		},
		{
			name:  "succeeds with exclude",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", ":", "12", "someFlag"},
			},
			ctxLines: 0,
			want:     []grepResultLine{},
			exclude:  ".*",
		},
		{
			name:  "succeeds with no LineText lines",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", ":", "12", "someFlag"},
			},
			ctxLines: -1,
			want: []grepResultLine{
				{Path: "flags.txt", LineNum: 12, FlagKeys: []string{"someFlag"}},
			},
		},
		{
			name:  "succeeds with multiple references",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", ":", "12", "someFlag"},
				{"", "path/flags.txt", ":", "12", "someFlag anotherFlag"},
			},
			ctxLines: 0,
			want: []grepResultLine{
				{Path: "flags.txt", LineNum: 12, LineText: "someFlag", FlagKeys: []string{"someFlag"}},
				{Path: "path/flags.txt", LineNum: 12, LineText: "someFlag anotherFlag", FlagKeys: []string{"someFlag", "anotherFlag"}},
			},
		},
		{
			name:  "succeeds with extra LineText lines",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", "-", "11", "not a flag key line"},
				{"", "flags.txt", ":", "12", "someFlag"},
				{"", "flags.txt", "-", "13", "not a flag key line"},
			},
			ctxLines: 1,
			want: []grepResultLine{
				{Path: "flags.txt", LineNum: 11, LineText: "not a flag key line"},
				{Path: "flags.txt", LineNum: 12, LineText: "someFlag", FlagKeys: []string{"someFlag"}},
				{Path: "flags.txt", LineNum: 13, LineText: "not a flag key line"},
			},
		},
		{
			name:  "succeeds with extra LineText lines and multiple flags",
			flags: []string{"someFlag", "anotherFlag"},
			grepResult: [][]string{
				{"", "flags.txt", "-", "11", "not a flag key line"},
				{"", "flags.txt", ":", "12", "someFlag"},
				{"", "flags.txt", "-", "13", "not a flag key line"},
				{"", "flags.txt", ":", "14", "anotherFlag"},
				{"", "flags.txt", "-", "15", "not a flag key line"},
			},
			ctxLines: 1,
			want: []grepResultLine{
				{Path: "flags.txt", LineNum: 11, LineText: "not a flag key line"},
				{Path: "flags.txt", LineNum: 12, LineText: "someFlag", FlagKeys: []string{"someFlag"}},
				{Path: "flags.txt", LineNum: 13, LineText: "not a flag key line"},
				{Path: "flags.txt", LineNum: 14, LineText: "anotherFlag", FlagKeys: []string{"anotherFlag"}},
				{Path: "flags.txt", LineNum: 15, LineText: "not a flag key line"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex, err := regexp.Compile(tt.exclude)
			require.NoError(t, err)
			got := generateReferencesFromGrep(tt.flags, tt.grepResult, tt.ctxLines, ex)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_findReferencedFlags(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want []string
	}{
		{
			name: "finds a flag",
			ref:  "line contains someFlag",
			want: []string{"someFlag"},
		},
		{
			name: "finds multiple flags",
			ref:  "line contains someFlag and anotherFlag",
			want: []string{"someFlag", "anotherFlag"},
		},
		{
			name: "finds no flags",
			ref:  "line contains no flags",
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findReferencedFlags(tt.ref, []string{"someFlag", "anotherFlag"})
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_makeReferenceHunksReps(t *testing.T) {
	projKey := "test"

	tests := []struct {
		name string
		refs grepResultLines
		want []ld.ReferenceHunksRep
	}{
		{
			name: "no references",
			refs: grepResultLines{},
			want: []ld.ReferenceHunksRep{},
		},
		{
			name: "single path, single reference with context lines",
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.ReferenceHunksRep{
				ld.ReferenceHunksRep{
					Path: "a/b",
					Hunks: []ld.HunkRep{
						ld.HunkRep{
							Offset:  5,
							Lines:   "context -1\nflag-1\ncontext +1\n",
							ProjKey: projKey,
							FlagKey: "flag-1",
						},
					},
				},
			},
		},
		{
			name: "multiple paths, single reference with context lines",
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  1,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/c/d",
					LineNum:  10,
					LineText: "flag-2",
					FlagKeys: []string{"flag-2"},
				},
			},
			want: []ld.ReferenceHunksRep{
				ld.ReferenceHunksRep{
					Path: "a/b",
					Hunks: []ld.HunkRep{
						ld.HunkRep{
							Offset:  1,
							Lines:   "flag-1\n",
							ProjKey: projKey,
							FlagKey: "flag-1",
						},
					},
				},
				ld.ReferenceHunksRep{
					Path: "a/c/d",
					Hunks: []ld.HunkRep{
						ld.HunkRep{
							Offset:  10,
							Lines:   "flag-2\n",
							ProjKey: projKey,
							FlagKey: "flag-2",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.refs.makeReferenceHunksReps(projKey, 1)

			sort.Sort(byPath(got))

			require.Equal(t, tt.want, got)
		})
	}
}

// TODO: test empty case?
func Test_makeHunkReps(t *testing.T) {
	projKey := "test"

	tests := []struct {
		name     string
		ctxLines int
		refs     grepResultLines
		want     []ld.HunkRep
	}{
		{
			name:     "single reference with context lines",
			ctxLines: 1,
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.HunkRep{
				ld.HunkRep{
					Offset:  5,
					Lines:   "context -1\nflag-1\ncontext +1\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
			},
		},
		{
			name:     "multiple references, single flag, one hunk",
			ctxLines: 1,
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "context inner",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  8,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  9,
					LineText: "context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.HunkRep{
				ld.HunkRep{
					Offset:  5,
					Lines:   "context -1\nflag-1\ncontext inner\nflag-1\ncontext +1\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
			},
		},
		{
			name:     "multiple references, single flag, multiple hunks",
			ctxLines: 1,
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "a context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "a flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "a context +1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  9,
					LineText: "b context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  10,
					LineText: "b flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  11,
					LineText: "b context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.HunkRep{
				ld.HunkRep{
					Offset:  5,
					Lines:   "a context -1\na flag-1\na context +1\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
				ld.HunkRep{
					Offset:  9,
					Lines:   "b context -1\nb flag-1\nb context +1\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
			},
		},
		{
			name:     "multiple consecutive references, multiple flags, multiple hunks",
			ctxLines: 1,
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "context inner",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  8,
					LineText: "flag-2",
					FlagKeys: []string{"flag-2"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  9,
					LineText: "context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.HunkRep{
				ld.HunkRep{
					Offset:  5,
					Lines:   "context -1\nflag-1\ncontext inner\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
				ld.HunkRep{
					Offset:  7,
					Lines:   "context inner\nflag-2\ncontext +1\n",
					ProjKey: projKey,
					FlagKey: "flag-2",
				},
			},
		},
		{
			name:     "multiple consecutive (non overlapping) references, multiple flags, multiple hunks",
			ctxLines: 1,
			refs: grepResultLines{
				grepResultLine{
					Path:     "a/b",
					LineNum:  5,
					LineText: "a context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  6,
					LineText: "a flag-1",
					FlagKeys: []string{"flag-1"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  7,
					LineText: "a context +1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  8,
					LineText: "b context -1",
					FlagKeys: []string{},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  9,
					LineText: "b flag-2",
					FlagKeys: []string{"flag-2"},
				},
				grepResultLine{
					Path:     "a/b",
					LineNum:  10,
					LineText: "b context +1",
					FlagKeys: []string{},
				},
			},
			want: []ld.HunkRep{
				ld.HunkRep{
					Offset:  5,
					Lines:   "a context -1\na flag-1\na context +1\n",
					ProjKey: projKey,
					FlagKey: "flag-1",
				},
				ld.HunkRep{
					Offset:  8,
					Lines:   "b context -1\nb flag-2\nb context +1\n",
					ProjKey: projKey,
					FlagKey: "flag-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupedResults := tt.refs.aggregateByPath()

			fileGrepResults, ok := groupedResults["a/b"]

			require.True(t, ok)

			got := fileGrepResults.makeHunkReps(projKey, tt.ctxLines)

			sort.Sort(byOffset(got))

			require.Equal(t, tt.want, got)
		})
	}
}

func Test_groupIntoPathMap(t *testing.T) {
	grepResultPathALine1 := grepResultLine{
		Path:     "a",
		LineNum:  1,
		LineText: "flag-1",
		FlagKeys: []string{"flag-1"},
	}

	grepResultPathALine2 := grepResultLine{
		Path:     "a",
		LineNum:  2,
		LineText: "flag-2",
		FlagKeys: []string{"flag-2"},
	}

	grepResultPathBLine1 := grepResultLine{
		Path:     "b",
		LineNum:  1,
		LineText: "flag-3",
		FlagKeys: []string{"flag-3"},
	}
	grepResultPathBLine2 := grepResultLine{
		Path:     "b",
		LineNum:  2,
		LineText: "flag-2",
		FlagKeys: []string{"flag-4"},
	}

	lines := grepResultLines{
		grepResultPathALine1,
		grepResultPathALine2,
		grepResultPathBLine1,
		grepResultPathBLine2,
	}

	pathMap := lines.aggregateByPath()

	aRefs, ok := pathMap["a"]
	require.True(t, ok)

	aRefMap := aRefs.flagReferenceMap
	require.Equal(t, len(aRefMap), 2)

	require.Contains(t, aRefMap, "flag-1")
	require.Contains(t, aRefMap, "flag-2")

	aLines := aRefs.fileGrepResultLines
	require.Equal(t, aLines.Len(), 2)
	require.Equal(t, aLines.Front().Value, grepResultPathALine1)
	require.Equal(t, aLines.Back().Value, grepResultPathALine2)

	bRefs, ok := pathMap["b"]
	require.True(t, ok)

	bRefMap := bRefs.flagReferenceMap
	require.Equal(t, len(aRefMap), 2)

	require.Contains(t, bRefMap, "flag-3")
	require.Contains(t, bRefMap, "flag-4")

	bLines := bRefs.fileGrepResultLines
	require.Equal(t, bLines.Len(), 2)
	require.Equal(t, bLines.Front().Value, grepResultPathBLine1)
	require.Equal(t, bLines.Back().Value, grepResultPathBLine2)
}
