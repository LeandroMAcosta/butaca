package store

import (
	"path/filepath"
	"testing"
)

// A non-ASCII title is the case that breaks byte-counted prefixes: substr()
// counts characters, so the prefix must be measured the same way.
func TestMoveItemPathRewritesFilesAndSubtitles(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "butaca.db"))
	if err != nil {
		t.Fatal(err)
	}
	from, to := "/m/movies/Amélie (2001)", "/m/documentaries/Amélie (2001)"
	id, err := s.AddItem(&Item{Kind: "movie", Title: "Amélie", Year: 2001, Path: from, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	fid, err := s.AddFile(&File{ItemID: id, Path: from + "/Amélie (2001).mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSubtitle(fid, "es", from+"/Amélie (2001).es.srt"); err != nil {
		t.Fatal(err)
	}
	// Another item whose folder merely starts with the same characters.
	other, _ := s.AddItem(&Item{Kind: "movie", Title: "Amélie 2", Path: from + " 2", Monitored: true})
	if _, err := s.AddFile(&File{ItemID: other, Path: from + " 2/x.mkv"}); err != nil {
		t.Fatal(err)
	}

	if err := s.MoveItemPath(id, from, to); err != nil {
		t.Fatal(err)
	}

	it, _ := s.GetItem(id)
	if it.Path != to {
		t.Errorf("item path = %q, want %q", it.Path, to)
	}
	files, _ := s.FilesForItem(id)
	if len(files) != 1 || files[0].Path != to+"/Amélie (2001).mkv" {
		t.Errorf("file path = %+v", files)
	}
	var sub string
	if err := s.db.QueryRow(`SELECT path FROM subtitles WHERE file_id = ?`, fid).Scan(&sub); err != nil {
		t.Fatal(err)
	}
	if sub != to+"/Amélie (2001).es.srt" {
		t.Errorf("subtitle path = %q", sub)
	}
	untouched, _ := s.FilesForItem(other)
	if untouched[0].Path != from+" 2/x.mkv" {
		t.Errorf("moved another item's file: %q", untouched[0].Path)
	}
}
