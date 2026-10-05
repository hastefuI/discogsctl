// Package output renders Discogs values as text for a terminal or as JSON for
// another program.
//
// JSON is the Discogs body itself, decoded and re-encoded, with no envelope:
// a single resource prints as an object and a listing prints as the array of
// its items. Text is an aligned table for a listing and a labelled block for a
// single resource.
package output

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/dump"
)

const (
	// FormatText prints tables and labelled blocks meant for a terminal.
	FormatText = "text"
	// FormatJSON prints indented JSON meant to be piped into another tool.
	FormatJSON = "json"
)

// ValidFormats returns the formats Write accepts.
func ValidFormats() []string {
	return []string{FormatText, FormatJSON}
}

// IsValidFormat reports whether format is one of ValidFormats.
func IsValidFormat(format string) bool {
	return slices.Contains(ValidFormats(), format)
}

// Write renders v to w in format. FormatText has a layout for each Discogs
// type and falls back to JSON for anything else.
func Write(w io.Writer, format string, v any) error {
	switch format {
	case FormatJSON:
		return writeJSON(w, v)
	case FormatText:
		return writeText(w, v)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeText(w io.Writer, v any) error {
	switch v := v.(type) {
	case *api.Release:
		return writeRelease(w, v)
	case *api.CollectionInstance:
		return writeBlock(w, []field{
			{"Instance", itoa(v.InstanceID)},
			{"URL", v.ResourceURL},
		})
	case *api.UserRating:
		return writeBlock(w, []field{
			{"Release", itoa(v.ReleaseID)},
			{"Username", v.Username},
			{"Rating", rating(v.Rating)},
		})
	case *api.ReleaseRating:
		return writeBlock(w, []field{
			{"Release", itoa(v.ReleaseID)},
			{"Average", strconv.FormatFloat(v.Rating.Average, 'f', 2, 64)},
			{"Votes", itoa(v.Rating.Count)},
		})
	case *api.Master:
		return writeMaster(w, v)
	case *api.Submissions:
		return writeSubmissions(w, v)
	case []api.List:
		return writeTable(w, "ID\tCHANGED\tPUBLIC\tNAME", v, func(l api.List) []string {
			return []string{itoa(l.ID), date(l.DateChanged), yesNo(l.Public), l.Name}
		})
	case *api.List:
		return writeList(w, v)
	case []api.Release:
		return writeTable(w, "ID\tADDED\tYEAR\tARTIST\tTITLE\tFORMAT", v, func(r api.Release) []string {
			return []string{itoa(r.ID), date(r.DateAdded), year(r.Year), artists(r.Artists), r.Title, formats(r.Formats)}
		})
	case []api.MasterVersion:
		return writeTable(w, "ID\tTITLE\tLABEL\tCATNO\tCOUNTRY\tYEAR\tFORMAT", v, func(m api.MasterVersion) []string {
			return []string{itoa(m.ID), m.Title, m.Label, m.Catno, m.Country, m.Released, strings.Join(slices.Concat(m.MajorFormats, []string{m.Format}), ", ")}
		})
	case *api.Artist:
		return writeArtist(w, v)
	case []api.ArtistRelease:
		return writeTable(w, "ID\tTYPE\tYEAR\tARTIST\tTITLE\tROLE", v, func(r api.ArtistRelease) []string {
			return []string{itoa(r.ID), r.Type, year(r.Year), r.Artist, r.Title, r.Role}
		})
	case *api.Label:
		return writeLabel(w, v)
	case []api.LabelRelease:
		return writeTable(w, "ID\tCATNO\tYEAR\tARTIST\tTITLE\tFORMAT", v, func(r api.LabelRelease) []string {
			return []string{itoa(r.ID), r.Catno, year(r.Year), r.Artist, r.Title, r.Format}
		})
	case []api.SearchResult:
		return writeTable(w, "ID\tTYPE\tYEAR\tTITLE\tCOUNTRY\tFORMAT\tCATNO", v, func(r api.SearchResult) []string {
			return []string{itoa(r.ID), r.Type, r.Year, r.Title, r.Country, strings.Join(r.Format, ", "), r.Catno}
		})
	case *api.Identity:
		return writeBlock(w, []field{
			{"ID", itoa(v.ID)},
			{"Username", v.Username},
			{"Application", v.ConsumerName},
			{"URL", v.ResourceURL},
		})
	case *api.User:
		return writeUser(w, v)
	case []api.Folder:
		return writeTable(w, "ID\tNAME\tCOUNT", v, func(f api.Folder) []string {
			return []string{itoa(f.ID), f.Name, itoa(f.Count)}
		})
	case []api.CollectionItem:
		return writeTable(w, "ID\tARTIST\tTITLE\tYEAR\tFORMAT\tRATING\tADDED", v, func(i api.CollectionItem) []string {
			b := i.BasicInformation
			return []string{itoa(i.ID), artists(b.Artists), b.Title, year(b.Year), formats(b.Formats), rating(i.Rating), date(i.DateAdded)}
		})
	case *api.CollectionValue:
		return writeBlock(w, []field{
			{"Minimum", v.Minimum},
			{"Median", v.Median},
			{"Maximum", v.Maximum},
		})
	case *api.Want:
		b := v.BasicInformation
		return writeBlock(w, []field{
			{"ID", itoa(v.ID)},
			{"Artists", artists(b.Artists)},
			{"Title", b.Title},
			{"Year", year(b.Year)},
			{"Formats", formats(b.Formats)},
			{"Rating", rating(v.Rating)},
			{"Notes", oneLine(v.Notes)},
			{"Added", date(v.DateAdded)},
		})
	case []api.Want:
		return writeTable(w, "ID\tARTIST\tTITLE\tYEAR\tFORMAT\tRATING\tADDED", v, func(want api.Want) []string {
			b := want.BasicInformation
			return []string{itoa(want.ID), artists(b.Artists), b.Title, year(b.Year), formats(b.Formats), rating(want.Rating), date(want.DateAdded)}
		})
	case []api.Order:
		return writeTable(w, "ID\tCREATED\tSTATUS\tBUYER\tITEMS\tTOTAL", v, func(o api.Order) []string {
			return []string{o.ID, date(o.Created), o.Status, o.Buyer.Username, itoa(len(o.Items)), price(o.Total)}
		})
	case []api.Listing:
		return writeTable(w, "ID\tSTATUS\tCONDITION\tPRICE\tPOSTED\tRELEASE", v, func(l api.Listing) []string {
			return []string{itoa(l.ID), l.Status, conditions(l.Condition, l.SleeveCondition), price(l.Price), date(l.Posted), l.Release.Description}
		})
	case *api.Order:
		return writeOrder(w, v)
	case []api.OrderMessage:
		return writeOrderMessages(w, v)
	case *api.PriceSuggestions:
		return writeTable(w, "CONDITION\tPRICE", byCondition(v.Prices), func(c string) []string {
			return []string{c, price(v.Prices[c])}
		})
	case *api.MarketplaceStats:
		return writeMarketplaceStats(w, v)
	case []dump.Fetched:
		return writeFetched(w, v)
	case []dump.Verified:
		return writeVerified(w, v)
	case []dump.Dump:
		return writeTable(w, "ID\tDATE\tTYPES\tCHECKSUM", v, func(d dump.Dump) []string {
			_, ok := d.Checksum()
			return []string{d.ID, d.Date, strings.Join(d.Types(), ","), yesNo(ok)}
		})
	default:
		return writeJSON(w, v)
	}
}

func writeRelease(w io.Writer, r *api.Release) error {
	fields := []field{
		{"ID", itoa(r.ID)},
		{"Title", r.Title},
		{"Artists", artists(r.Artists)},
		{"Released", cmp.Or(r.Released, year(r.Year))},
		{"Country", r.Country},
		{"Labels", labels(r.Labels)},
		{"Formats", formats(r.Formats)},
		{"Genres", strings.Join(r.Genres, ", ")},
		{"Styles", strings.Join(r.Styles, ", ")},
		{"Master", id(r.MasterID)},
		{"Rating", votes(r.Community.Rating)},
		{"Have", itoa(r.Community.Have)},
		{"Want", itoa(r.Community.Want)},
		{"For sale", forSale(r.NumForSale, r.LowestPrice)},
		{"URL", r.URI},
	}
	if err := writeBlock(w, fields); err != nil {
		return err
	}
	return writeTracklist(w, r.Tracklist)
}

func writeMaster(w io.Writer, m *api.Master) error {
	fields := []field{
		{"ID", itoa(m.ID)},
		{"Title", m.Title},
		{"Artists", artists(m.Artists)},
		{"Year", year(m.Year)},
		{"Main release", id(m.MainRelease)},
		{"Genres", strings.Join(m.Genres, ", ")},
		{"Styles", strings.Join(m.Styles, ", ")},
		{"For sale", forSale(m.NumForSale, m.LowestPrice)},
		{"URL", m.URI},
	}
	if err := writeBlock(w, fields); err != nil {
		return err
	}
	return writeTracklist(w, m.Tracklist)
}

func writeArtist(w io.Writer, a *api.Artist) error {
	return writeBlock(w, []field{
		{"ID", itoa(a.ID)},
		{"Name", a.Name},
		{"Real name", a.RealName},
		{"Members", refs(a.Members)},
		{"Groups", refs(a.Groups)},
		{"Variations", strings.Join(a.NameVariations, ", ")},
		{"Links", strings.Join(a.URLs, " ")},
		{"URL", a.URI},
		{"Profile", oneLine(a.Profile)},
	})
}

func writeLabel(w io.Writer, l *api.Label) error {
	parent := ""
	if l.ParentLabel != nil {
		parent = l.ParentLabel.Name
	}
	sublabels := make([]string, len(l.Sublabels))
	for i, s := range l.Sublabels {
		sublabels[i] = s.Name
	}
	return writeBlock(w, []field{
		{"ID", itoa(l.ID)},
		{"Name", l.Name},
		{"Parent", parent},
		{"Sublabels", strings.Join(sublabels, ", ")},
		{"Links", strings.Join(l.URLs, " ")},
		{"URL", l.URI},
		{"Profile", oneLine(l.Profile)},
	})
}

func writeUser(w io.Writer, u *api.User) error {
	ratedAvg := ""
	if u.ReleasesRated > 0 {
		ratedAvg = strconv.FormatFloat(u.RatingAvg, 'f', 2, 64)
	}
	return writeBlock(w, []field{
		{"ID", itoa(u.ID)},
		{"Username", u.Username},
		{"Name", u.Name},
		{"Email", u.Email},
		{"Location", u.Location},
		{"Home page", u.HomePage},
		{"Registered", date(u.Registered)},
		{"Rank", strconv.FormatFloat(u.Rank, 'f', -1, 64)},
		{"Rating avg", ratedAvg},
		{"Contributed", itoa(u.ReleasesContributed)},
		{"Rated", itoa(u.ReleasesRated)},
		{"Buyer", feedback(u.BuyerRating, u.BuyerNumRatings)},
		{"Seller", feedback(u.SellerRating, u.SellerNumRatings)},
		{"Collection", private(u.NumCollection)},
		{"Wantlist", private(u.NumWantlist)},
		{"For sale", itoa(u.NumForSale)},
		{"Lists", itoa(u.NumLists)},
		{"Currency", u.CurrAbbr},
		{"URL", u.URI},
		{"Profile", oneLine(u.Profile)},
	})
}

// feedback is a marketplace rating as a percentage and the number of ratings
// behind it, and empty when there are none.
func feedback(percent float64, n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f%% (%d ratings)", percent, n)
}

// private is a count Discogs leaves out when the list is private.
func private(n *int) string {
	if n == nil {
		return "private"
	}
	return itoa(*n)
}

// writeSubmissions prints one table of artists, labels and releases, each row
// naming its kind.
func writeSubmissions(w io.Writer, s *api.Submissions) error {
	type row struct{ kind, id, name, quality string }
	var rows []row
	for _, a := range s.Artists {
		rows = append(rows, row{"artist", itoa(a.ID), a.Name, a.DataQuality})
	}
	for _, l := range s.Labels {
		rows = append(rows, row{"label", itoa(l.ID), l.Name, l.DataQuality})
	}
	for _, r := range s.Releases {
		name := strings.TrimSpace(r.Title)
		if a := artists(r.Artists); a != "" {
			name = a + " - " + name
		}
		rows = append(rows, row{"release", itoa(r.ID), name, r.DataQuality})
	}
	return writeTable(w, "TYPE\tID\tNAME\tQUALITY", rows, func(r row) []string {
		return []string{r.kind, r.id, r.name, r.quality}
	})
}

func writeList(w io.Writer, l *api.List) error {
	if err := writeBlock(w, []field{
		{"ID", itoa(l.ID)},
		{"Name", l.Name},
		{"Owner", l.User.Username},
		{"Public", yesNo(l.Public)},
		{"Added", date(l.DateAdded)},
		{"Changed", date(l.DateChanged)},
		{"Items", itoa(len(l.Items))},
		{"URL", l.URI},
		{"Description", oneLine(l.Description)},
	}); err != nil {
		return err
	}
	if len(l.Items) == 0 {
		return nil
	}
	fmt.Fprintln(w, "\nItems:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, it := range l.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", it.Type, itoa(it.ID), cell(it.DisplayTitle), cell(it.Comment))
	}
	return tw.Flush()
}

// writeOrder leaves out the shipping address, which is the buyer's personal
// data, so the view can be shown or pasted without it.
func writeOrder(w io.Writer, o *api.Order) error {
	shipping := ""
	if o.Shipping.Currency != "" {
		shipping = price(api.Price{Value: o.Shipping.Value, Currency: o.Shipping.Currency})
		if o.Shipping.Method != "" {
			shipping += " (" + o.Shipping.Method + ")"
		}
	}
	tracking := ""
	if t := o.Tracking; t != nil {
		tracking = strings.TrimSpace(t.Carrier + " " + t.Number)
	}
	if err := writeBlock(w, []field{
		{"ID", o.ID},
		{"Status", o.Status},
		{"Created", date(o.Created)},
		{"Last activity", date(o.LastActivity)},
		{"Buyer", o.Buyer.Username},
		{"Total", price(o.Total)},
		{"Shipping", shipping},
		{"Fee", price(o.Fee)},
		{"Tracking", tracking},
		{"Archived", yesNo(o.Archived)},
		{"Next status", strings.Join(o.NextStatus, ", ")},
		{"Instructions", oneLine(o.AdditionalInstructions)},
		{"URL", o.URI},
	}); err != nil {
		return err
	}
	if len(o.Items) == 0 {
		return nil
	}
	fmt.Fprintln(w, "\nItems:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, it := range o.Items {
		condition := strings.Join(slices.DeleteFunc([]string{it.MediaCondition, it.SleeveCondition}, func(s string) bool { return s == "" }), " / ")
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", id(it.Release.ID), cell(it.Release.Description), condition, price(it.Price))
	}
	return tw.Flush()
}

// writeOrderMessages prints each entry as a heading line of time, type and
// who, then the full text indented below it. A table would cut messages off.
func writeOrderMessages(w io.Writer, msgs []api.OrderMessage) error {
	if len(msgs) == 0 {
		fmt.Fprintln(w, "No results.")
		return nil
	}
	for i, m := range msgs {
		if i > 0 {
			fmt.Fprintln(w)
		}
		who := ""
		switch {
		case m.From != nil:
			who = m.From.Username
		case m.Actor != nil:
			who = m.Actor.Username
		}
		fmt.Fprintln(w, strings.TrimSpace(strings.Join([]string{timestamp(m.Timestamp), m.Type, who}, "  ")))
		for line := range strings.Lines(strings.TrimSpace(strings.ReplaceAll(m.Message, "\r\n", "\n"))) {
			fmt.Fprintln(w, "  "+strings.TrimRight(line, "\n"))
		}
	}
	return nil
}

// timestamp shortens a Discogs time such as 2015-06-02T13:17:54-07:00 to
// 2015-06-02 13:17, in the offset Discogs sent, and leaves anything else as
// it is.
func timestamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04")
}

// writeMarketplaceStats leaves out the count and price Discogs sends as null.
// No count means none for sale, unless the release is blocked from sale.
func writeMarketplaceStats(w io.Writer, s *api.MarketplaceStats) error {
	forSale, lowest := "", ""
	switch {
	case s.NumForSale != nil:
		forSale = itoa(*s.NumForSale)
	case !s.BlockedFromSale:
		forSale = "0"
	}
	if s.LowestPrice != nil {
		lowest = price(*s.LowestPrice)
	}
	return writeBlock(w, []field{
		{"For sale", forSale},
		{"Lowest", lowest},
		{"Blocked", yesNo(s.BlockedFromSale)},
	})
}

// writeFetched is a table like writeTable, but never truncates the path,
// which is there to be copied.
func writeFetched(w io.Writer, files []dump.Fetched) error {
	if len(files) == 0 {
		fmt.Fprintln(w, "No results.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tSTATUS\tPATH")
	for _, f := range files {
		status := "downloaded"
		if f.Skipped {
			status = "already present"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Type, status, f.Path)
	}
	return tw.Flush()
}

// writeVerified prints one line per file, with the reason after a failure.
// Nothing is truncated: the path is there to be copied and the reason to be
// read.
func writeVerified(w io.Writer, files []dump.Verified) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tPATH")
	for _, f := range files {
		status := "ok"
		if !f.OK {
			status = "FAILED"
		}
		fmt.Fprintf(tw, "%s\t%s\n", status, f.Path)
		if f.Error != "" {
			fmt.Fprintf(tw, "\t  %s\n", f.Error)
		}
	}
	return tw.Flush()
}

func writeTracklist(w io.Writer, tracks []api.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	fmt.Fprintln(w, "\nTracklist:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, t := range tracks {
		if t.Type == "heading" {
			fmt.Fprintf(tw, "  \t%s\t\n", cell(t.Title))
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", t.Position, cell(t.Title), t.Duration)
	}
	return tw.Flush()
}

type field struct{ label, value string }

// writeBlock prints one "Label: value" line per field, leaving out empty
// values.
func writeBlock(w io.Writer, fields []field) error {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
	for _, f := range fields {
		if f.value != "" {
			fmt.Fprintf(tw, "%s:\t%s\n", f.label, f.value)
		}
	}
	return tw.Flush()
}

func writeTable[T any](w io.Writer, header string, items []T, row func(T) []string) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "No results.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header)
	for _, item := range items {
		cells := row(item)
		for i, c := range cells {
			cells[i] = cell(c)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

const maxCell = 40

// cell flattens s to one line and truncates it to maxCell characters, so one
// long title does not push every column off the screen.
func cell(s string) string {
	s = oneLine(s)
	if r := []rune(s); len(r) > maxCell {
		return string(r[:maxCell-1]) + "…"
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// artists joins credits the way Discogs displays them, using the name
// variation when there is one and the join text between names.
func artists(credits []api.ArtistCredit) string {
	var b strings.Builder
	for i, a := range credits {
		b.WriteString(cmp.Or(a.ANV, a.Name))
		if i == len(credits)-1 {
			break
		}
		switch j := strings.TrimSpace(a.Join); j {
		case "", ",":
			b.WriteString(", ")
		default:
			b.WriteString(" " + j + " ")
		}
	}
	return b.String()
}

func labels(credits []api.LabelCredit) string {
	parts := make([]string, len(credits))
	for i, l := range credits {
		parts[i] = l.Name
		if l.Catno != "" {
			parts[i] += " (" + l.Catno + ")"
		}
	}
	return strings.Join(parts, ", ")
}

func formats(fs []api.Format) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		desc := append([]string{f.Name}, f.Descriptions...)
		if f.Text != "" {
			desc = append(desc, f.Text)
		}
		if q, err := strconv.Atoi(f.Qty); err == nil && q > 1 {
			desc[0] = f.Qty + "×" + f.Name
		}
		parts = append(parts, strings.Join(desc, ", "))
	}
	return strings.Join(parts, " + ")
}

func refs(rs []api.ArtistRef) string {
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

func votes(r api.Rating) string {
	if r.Count == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f (%d votes)", r.Average, r.Count)
}

func forSale(n int, lowest *float64) string {
	if n == 0 {
		return ""
	}
	if lowest == nil {
		return itoa(n)
	}
	return fmt.Sprintf("%d, from %.2f", n, *lowest)
}

func rating(r int) string {
	if r == 0 {
		return "-"
	}
	return itoa(r)
}

// date keeps the date part of a Discogs timestamp such as
// 2017-06-22T15:25:55-07:00.
func date(s string) string {
	d, _, _ := strings.Cut(s, "T")
	return d
}

func year(y int) string {
	if y == 0 {
		return ""
	}
	return itoa(y)
}

func id(n int) string {
	if n == 0 {
		return ""
	}
	return itoa(n)
}

// conditions shortens media and sleeve grades such as "Very Good Plus (VG+)"
// to the abbreviation in brackets, as "VG / VG+".
func conditions(grades ...string) string {
	var short []string
	for _, g := range grades {
		if open, end := strings.LastIndex(g, "("), strings.LastIndex(g, ")"); open >= 0 && end > open {
			g = g[open+1 : end]
		}
		if g != "" {
			short = append(short, g)
		}
	}
	return strings.Join(short, " / ")
}

// byCondition returns the conditions in prices, best first, followed by any
// Discogs adds that api.Conditions does not know, sorted.
func byCondition(prices map[string]api.Price) []string {
	var known, other []string
	for _, c := range api.Conditions {
		if _, ok := prices[c]; ok {
			known = append(known, c)
		}
	}
	for c := range prices {
		if !slices.Contains(api.Conditions, c) {
			other = append(other, c)
		}
	}
	slices.Sort(other)
	return append(known, other...)
}

// price is an amount and its currency, such as 42.00 USD, and empty when
// Discogs sent none.
func price(p api.Price) string {
	if p.Currency == "" {
		return ""
	}
	return fmt.Sprintf("%.2f %s", p.Value, p.Currency)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func itoa(n int) string { return strconv.Itoa(n) }
