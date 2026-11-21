package markdown

import (
	"slices"
	"strings"
	"testing"
)

func TestFenceLanguages(t *testing.T) {
	cases := []struct {
		name     string
		markdown string
		want     []string
	}{
		{
			name:     "backtick fence with a language",
			markdown: "```go\nfunc main() {}\n```\n",
			want:     []string{"go"},
		},
		{
			name:     "tilde fence",
			markdown: "~~~python\nprint(1)\n~~~\n",
			want:     []string{"python"},
		},
		{
			name:     "longer fence than the one in its code",
			markdown: "````go\n```\nnested\n````\n",
			want:     []string{"go"},
		},
		{
			name:     "language-less fence",
			markdown: "```\nplain\n```\n",
			want:     []string{""},
		},
		{
			name:     "quoted fence",
			markdown: "> ```go\n> fmt.Println()\n> ```\n",
			want:     []string{"go"},
		},
		{
			name:     "fence in a nested quotation",
			markdown: "> > ~~~js\n> > x\n> > ~~~\n",
			want:     []string{"js"},
		},
		{
			name:     "fence indented four spaces is code",
			markdown: "text\n\n    ```go\n    code\n    ```\n",
			want:     nil,
		},
		{
			name:     "fence inside a block is the block's code",
			markdown: "```\n```go\n```\n",
			want:     []string{""},
		},
		{
			name:     "several blocks in order",
			markdown: "```go\na\n```\n\n~~~\nb\n~~~\n",
			want:     []string{"go", ""},
		},
		{
			name:     "info string attributes after the language",
			markdown: "```go title=\"main\"\npackage main\n```\n",
			want:     []string{"go"},
		},
		{
			name:     "unclosed fence still names its language",
			markdown: "intro\n\n```rust\nfn main() {}\n",
			want:     []string{"rust"},
		},
		{
			name:     "no fences",
			markdown: "# A page\n\nWithout code.\n",
			want:     nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FenceLanguages(c.markdown); !slices.Equal(got, c.want) {
				t.Errorf("FenceLanguages = %q, want %q", got, c.want)
			}
		})
	}
}

func TestApplyFenceLanguages(t *testing.T) {
	const page = "Intro.\n\n```\na := 1\n```\n\n~~~\nb := 2\n~~~\n\n````\n```\ninner\n````\n"
	langs := []string{"go", "python", ""}
	const want = "Intro.\n\n```go\na := 1\n```\n\n~~~python\nb := 2\n~~~\n\n````\n```\ninner\n````\n"
	got := ApplyFenceLanguages(page, langs)
	if got != want {
		t.Fatalf("ApplyFenceLanguages =\n%q\nwant\n%q", got, want)
	}
	if again := ApplyFenceLanguages(got, langs); again != got {
		t.Errorf("applying the languages twice changed the page:\n%q\n%q", got, again)
	}
	if scanned := FenceLanguages(got); !slices.Equal(scanned, langs) {
		t.Errorf("the applied languages scan as %q, want %q", scanned, langs)
	}
	// The code of each block, the closing fences and the blank lines between
	// the blocks are the writer's, untouched.
	for _, text := range []string{"a := 1\n", "b := 2\n", "```\n\n~~~", "\n````\n"} {
		if !strings.Contains(got, text) {
			t.Errorf("the page lost %q:\n%s", text, got)
		}
	}
	// A fence that already names a language keeps it.
	named := "```go\na := 1\n```\n"
	if kept := ApplyFenceLanguages(named, []string{"python"}); kept != named {
		t.Errorf("applyFenceLanguages renamed a fence: %q, want %q", kept, named)
	}
	// Nothing to add, nothing to split.
	if plain := ApplyFenceLanguages(page, nil); plain != page {
		t.Errorf("applying no languages changed the page: %q", plain)
	}
}

func TestReplaceTitle(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		title    string
		want     string
	}{
		{"atx h1", "# Old\n\nBody\n", "New", "# New\n\nBody\n"},
		{"keeps level", "### Old\n", "New", "### New\n"},
		{"after paragraphs", "intro\n\n## Old\n", "New", "intro\n\n## New\n"},
		{"no heading", "Body only\n", "New", "# New\n\nBody only\n"},
		{"empty document", "", "New", "# New\n\n"},
		{"after front matter", "---\ntitle: x\n---\n\n# Old\n", "New", "---\ntitle: x\n---\n\n# New\n"},
		{"before front matter's heading", "---\ntitle: x\n---\nBody\n", "New", "---\ntitle: x\n---\n# New\n\nBody\n"},
		{"skips a heading inside a fence", "```\n# Not\n```\n\n# Old\n", "New", "```\n# Not\n```\n\n# New\n"},
		{"skips an indented heading", "    # Not\n\n# Old\n", "New", "    # Not\n\n# New\n"},
		{"setext keeps its underline", "Old\n===\n\nBody\n", "New", "New\n===\n\nBody\n"},
		{"closing hashes go", "## Old ##\n", "New", "## New\n"},
		{"crlf", "## Old\r\n", "New", "## New\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceTitle(tt.markdown, tt.title); got != tt.want {
				t.Errorf("ReplaceTitle(%q, %q) = %q, want %q", tt.markdown, tt.title, got, tt.want)
			}
		})
	}
}
