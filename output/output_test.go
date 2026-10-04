package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.hasteful.org/discogsctl/api"
)

func TestJSONIsTheDiscogsBodyUnescaped(t *testing.T) {
	var items []api.LabelRelease
	body := `[{"id":1,"title":"Stars","artist":"R & S <Mix>","extra":true}]`
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatJSON, items); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, `"artist": "R & S <Mix>"`) || !strings.Contains(got, `"extra": true`) {
		t.Errorf("JSON output\n%s\nwant the body with every field and no HTML escaping", got)
	}
}

func TestTextTable(t *testing.T) {
	items := []api.Folder{{ID: 0, Name: "All", Count: 23}, {ID: 1, Name: "Uncategorized", Count: 20}}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, items); err != nil {
		t.Fatal(err)
	}
	want := "ID  NAME           COUNT\n0   All            23\n1   Uncategorized  20\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextEmptyListing(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, []api.Want{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "No results.\n" {
		t.Errorf("text output %q, want No results.", buf.String())
	}
}

func TestArtists(t *testing.T) {
	credits := []api.ArtistCredit{
		{Name: "Eric B.", Join: "&"},
		{Name: "Rakim (2)", ANV: "Rakim", Join: ","},
		{Name: "Someone"},
	}
	if got, want := artists(credits), "Eric B. & Rakim, Someone"; got != want {
		t.Errorf("artists = %q, want %q", got, want)
	}
}

func TestCellTruncatesByCharacter(t *testing.T) {
	s := strings.Repeat("ニ", maxCell+5)
	got := cell(s)
	if r := []rune(got); len(r) != maxCell || r[len(r)-1] != '…' {
		t.Errorf("cell(%d runes) = %d runes ending %q, want %d ending …", len([]rune(s)), len(r), r[len(r)-1], maxCell)
	}
}

func TestInvalidFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "yaml", 1); err == nil {
		t.Error("Write(yaml) succeeded, want error")
	}
}
