package cmd

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/dump"
)

func newDumpCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Work with the Discogs data dumps",
		Long: `Work with the monthly data dumps Discogs publishes at https://data.discogs.com:
XML exports of the artists, labels, masters and releases in the database,
under the CC0 licence. Reading them is not an API request, so no token is
needed and the API rate limit does not apply.`,
	}
	cmd.AddCommand(newDumpListCmd(cfg), newDumpFetchCmd(cfg), newDumpVerifyCmd(cfg))
	return cmd
}

func (cfg *config) dumpClient() (*dump.Client, error) {
	return dump.New(dump.Options{BaseURL: cfg.dumpURL, UserAgent: cfg.agent, Logger: cfg.logger})
}

func newDumpListCmd(cfg *config) *cobra.Command {
	var (
		year  int
		all   bool
		types []string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the published data dumps",
		Long: `List the data dumps published at data.discogs.com, newest first, with the
types each one has. Not every dump has every type. Without --year or --all,
the newest year is listed.

--type narrows the list to the dumps that have every type named, and each
dump to those files and its checksum. Without it, every dump and file is
listed.

A dump's ID is the date stamp in its file names, such as 20261001. No two
dumps share one, so the ID is how to refer to a dump.

Discogs publishes no machine-readable index, so the list is read from the
download links on the data.discogs.com listing pages. Sizes and times are as
that page shows them: the size is rounded and no time zone is given.`,
		Example: `  discogsctl dump list
  discogsctl dump list --year 2025
  discogsctl dump list --type releases
  discogsctl dump list --all --type masters,releases
  discogsctl dump list --output json | jq -r '.[0].id'
  discogsctl dump list --output json | jq -r '.[] | select(.id == "20261001") | .files[] | select(.type == "releases") | .url'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("year") && year < 1 {
				return fmt.Errorf("--year must be a year such as 2026, got %d", year)
			}
			want, err := dataTypes(types)
			if err != nil {
				return err
			}
			client, err := cfg.dumpClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			years := []int{year}
			newest := !all && !cmd.Flags().Changed("year")
			if !cmd.Flags().Changed("year") {
				if years, err = client.Years(ctx); err != nil {
					return err
				}
				if newest {
					years = years[max(0, len(years)-2):]
				}
			}

			// Years is oldest first and List is newest first within a year.
			// Without --year or --all, a year with no dumps of the types
			// asked for, such as early January, falls back to the year
			// before, and no further.
			dumps := []dump.Dump{}
			for _, y := range slices.Backward(years) {
				list, err := client.List(ctx, y)
				if err != nil {
					return err
				}
				for _, d := range list {
					if !d.Has(want...) {
						continue
					}
					if len(want) > 0 {
						d = d.Only(want...)
					}
					dumps = append(dumps, d)
				}
				if newest && len(dumps) > 0 {
					break
				}
			}
			return cfg.print(cmd, dumps)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "year to list (default the newest year with dumps)")
	cmd.Flags().BoolVar(&all, "all", false, "list every year, one request per year")
	bindTypeFlag(cmd, &types, "only dumps with every one of these types")
	cmd.MarkFlagsMutuallyExclusive("year", "all")
	return cmd
}

func newDumpFetchCmd(cfg *config) *cobra.Command {
	var (
		latest bool
		types  []string
		dir    string
	)
	cmd := &cobra.Command{
		Use:   "fetch [id]",
		Short: "Download a data dump and verify it",
		Long: `Download the files of one data dump into --dir and check each against the
SHA-256 in the dump's CHECKSUM file. Name the dump by its ID from dump list,
such as 20261001, or by the start of one, such as 202610. --latest takes the
newest dump that has a CHECKSUM file and every type asked for.

Each file is written to <name>.part and renamed only when its SHA-256
matches; otherwise it is deleted. A file already in --dir with the right
SHA-256 is kept, so an interrupted fetch can be run again. The CHECKSUM file
is saved beside them, for dump verify. Without --type,
every file the dump has is downloaded, about 11 GB for a recent dump.

Progress goes to stderr. Stdout lists the files saved.`,
		Example: `  discogsctl dump fetch --latest --type releases
  discogsctl dump fetch 20261001 --type labels,masters --dir ./dumps
  discogsctl dump fetch 202609 --output json | jq -r '.[].path'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if latest == (len(args) == 1) {
				return errors.New("give a dump ID, such as 20261001, or --latest")
			}
			want, err := dataTypes(types)
			if err != nil {
				return err
			}
			client, err := cfg.dumpClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			var d dump.Dump
			if latest {
				d, err = client.Latest(ctx, want...)
			} else {
				d, err = client.Find(ctx, args[0])
			}
			if err != nil {
				return err
			}
			if !d.Has(want...) {
				missing := slices.DeleteFunc(slices.Clone(want), func(t string) bool { return slices.Contains(d.Types(), t) })
				return fmt.Errorf("dump %s has no %s file", d.ID, strings.Join(missing, " or "))
			}
			if len(want) > 0 {
				d = d.Only(want...)
			}

			// Every file must be in the CHECKSUM file before any is
			// downloaded, so nothing is saved that cannot be verified.
			checksumFile, err := client.ChecksumFile(ctx, d)
			if err != nil {
				return err
			}
			sums, err := dump.ParseChecksums(bytes.NewReader(checksumFile))
			if err != nil {
				return err
			}
			var files []dump.File
			for _, f := range d.Files {
				if f.Type == dump.TypeChecksum {
					continue
				}
				if _, ok := sums[f.Name]; !ok {
					return fmt.Errorf("%s is not in the dump's CHECKSUM file, so it cannot be verified", f.Name)
				}
				files = append(files, f)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			// The CHECKSUM file is saved beside the data files, so dump verify
			// can check them later without a request.
			if sumFile, ok := d.Checksum(); ok {
				if err := writeFileAtomic(filepath.Join(dir, sumFile.Name), checksumFile); err != nil {
					return err
				}
			}

			stderr := cmd.ErrOrStderr()
			fetched := []dump.Fetched{}
			for _, f := range files {
				fmt.Fprintf(stderr, "Fetching %s (%s)\n", f.Name, cmp.Or(f.Size, "size unknown"))
				start := time.Now()
				r, err := client.Fetch(ctx, f, sums[f.Name], dir, progress(stderr))
				if err != nil {
					return err
				}
				if r.Skipped {
					fmt.Fprintf(stderr, "Kept %s, already present with the right SHA-256\n", r.Path)
				} else {
					fmt.Fprintf(stderr, "Saved %s in %s, SHA-256 verified\n", r.Path, time.Since(start).Round(time.Second))
				}
				fetched = append(fetched, r)
			}
			return cfg.print(cmd, fetched)
		},
	}
	cmd.Flags().BoolVar(&latest, "latest", false, "fetch the newest dump with a CHECKSUM file and every --type")
	bindTypeFlag(cmd, &types, "dump types to fetch, all required to exist")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory to save the files in, created if missing")
	return cmd
}

func newDumpVerifyCmd(cfg *config) *cobra.Command {
	var checksums string
	cmd := &cobra.Command{
		Use:   "verify <file>...",
		Short: "Check dump files against their CHECKSUM file",
		Long: `Check data dump files against the SHA-256 their dump's CHECKSUM file lists,
without downloading anything. Each file must keep its published name, such as
discogs_20260801_releases.xml.gz. The CHECKSUM file is the one beside it,
such as discogs_20260801_CHECKSUM.txt, which dump fetch saves, unless
--checksum names another.

Every file is checked and reported, and the exit status is 1 if any is
missing from its CHECKSUM file or does not match. Each file is read in full,
so a 10 GB file takes from a few seconds to a minute, depending on the disk.`,
		Example: `  discogsctl dump verify ./dumps/discogs_20260801_releases.xml.gz
  discogsctl dump verify ./dumps/*.xml.gz
  discogsctl dump verify discogs_20260201_releases.xml.gz --checksum ~/Downloads/discogs_20260201_CHECKSUM.txt`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results := make([]dump.Verified, 0, len(args))
			failed := 0
			for _, path := range args {
				fmt.Fprintf(cmd.ErrOrStderr(), "Checking %s\n", path)
				v, err := dump.Verify(path, checksums)
				if err != nil {
					failed++
				}
				results = append(results, v)
			}
			if err := cfg.print(cmd, results); err != nil {
				return err
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d files failed verification", failed, len(args))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&checksums, "checksum", "", "CHECKSUM file to check against (default the one beside each file)")
	return cmd
}

// writeFileAtomic writes data to a temporary file beside path and renames it
// into place, so path is never left half written.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// bindTypeFlag adds --type, shared by dump list and dump fetch.
func bindTypeFlag(cmd *cobra.Command, types *[]string, usage string) {
	cmd.Flags().StringSliceVar(types, "type", nil, usage+": "+strings.Join(dump.DataTypes, ", ")+" (default all)")
}

// progress returns a callback that prints how much of a download has arrived
// to w, at most every 10 seconds.
func progress(w io.Writer) func(done, total int64) {
	last := time.Now()
	return func(done, total int64) {
		if time.Since(last) < 10*time.Second {
			return
		}
		last = time.Now()
		if total > 0 {
			fmt.Fprintf(w, "  %s of %s (%d%%)\n", byteSize(done), byteSize(total), done*100/total)
			return
		}
		fmt.Fprintf(w, "  %s\n", byteSize(done))
	}
}

// byteSize formats n in decimal units, such as 10.5 GB.
func byteSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// dataTypes checks --type against dump.DataTypes, so a typo such as
// "release" is an error rather than an empty list.
func dataTypes(types []string) ([]string, error) {
	out := make([]string, 0, len(types))
	for _, t := range types {
		t = strings.ToLower(strings.TrimSpace(t))
		if !slices.Contains(dump.DataTypes, t) {
			return nil, fmt.Errorf("invalid --type %q, want one of %s", t, strings.Join(dump.DataTypes, ", "))
		}
		out = append(out, t)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
