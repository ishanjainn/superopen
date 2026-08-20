package memory

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	teachChunkTokens = 256
	teachOverlapTok  = 48
)

var teachSuffixes = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".rst": true, ".tex": true, ".bib": true,
	".csv": true, ".pdf": true, ".docx": true, ".pptx": true, ".xlsx": true, ".rtf": true,
	".epub": true, ".py": true, ".rs": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".go": true, ".java": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".rb": true,
	".php": true, ".swift": true, ".kt": true, ".sql": true, ".toml": true, ".yaml": true,
	".yml": true, ".json": true, ".ini": true, ".cfg": true, ".html": true, ".css": true,
	".xml": true, ".proto": true, ".sh": true, ".bash": true, ".zsh": true,
}

type TeachReport struct {
	Inserted       int       `json:"inserted"`
	Reinforced     int       `json:"reinforced"`
	Edges          int       `json:"edges"`
	RecallTested   int       `json:"recall_tested"`
	RecallVerified int       `json:"recall_verified"`
	Episodes       []Episode `json:"episodes,omitempty"`
}

func TeachFile(root, path, title string) (Episode, error) {
	report, err := TeachPath(root, path, title)
	if err != nil {
		return Episode{}, err
	}
	if len(report.Episodes) == 0 {
		return Episode{}, nil
	}
	return report.Episodes[0], nil
}

func TeachText(root, title, text string) (Episode, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return Episode{}, err
	}
	defer store.Close()
	report, err := store.studyText(title, text, nil)
	if err != nil {
		return Episode{}, err
	}
	if len(report.Episodes) == 0 {
		return Episode{}, nil
	}
	return report.Episodes[0], nil
}

func TeachPath(root, path, title string) (TeachReport, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return TeachReport{}, err
	}
	defer store.Close()
	return store.studyPath(path, title)
}

func (s *Store) studyPath(path, title string) (TeachReport, error) {
	info, err := os.Stat(path)
	if err != nil {
		return TeachReport{}, err
	}
	if info.IsDir() {
		var all TeachReport
		_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return err
			}
			if !teachSuffixes[strings.ToLower(filepath.Ext(p))] {
				return nil
			}
			rep, err := s.studyFile(p, "")
			if err != nil {
				return nil
			}
			all.Inserted += rep.Inserted
			all.Reinforced += rep.Reinforced
			all.Edges += rep.Edges
			all.RecallTested += rep.RecallTested
			all.RecallVerified += rep.RecallVerified
			all.Episodes = append(all.Episodes, rep.Episodes...)
			return nil
		})
		_ = s.ClusterTopics()
		return all, nil
	}
	rep, err := s.studyFile(path, title)
	if err != nil {
		return rep, err
	}
	_ = s.ClusterTopics()
	return rep, nil
}

func (s *Store) studyFile(path, title string) (TeachReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TeachReport{}, err
	}
	text := extractTeachText(path, data)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return s.studyText(title, text, []string{path})
}

func (s *Store) studyText(title, text string, files []string) (TeachReport, error) {
	chunks := chunkTeach(text)
	if len(chunks) == 0 && strings.TrimSpace(text) != "" {
		chunks = []string{text}
	}
	var report TeachReport
	var ids []int64
	prev := int64(0)
	for i, chunk := range chunks {
		label := title
		if len(chunks) > 1 {
			label = title + " (" + itoa(i+1) + ")"
		}
		ep, err := s.Capture(CaptureInput{
			Kind:   KindTeaching,
			Source: SourceTeach,
			Title:  label,
			Text:   chunk,
			Files:  files,
		})
		if err != nil {
			continue
		}
		ids = append(ids, ep.ID)
		report.Episodes = append(report.Episodes, ep)
		if prev != 0 {
			if err := s.addEdge(ep.ID, prev, EdgeTaughtFrom); err == nil {
				report.Edges++
			}
		}
		prev = ep.ID
	}
	if len(ids) == 0 {
		return report, nil
	}
	inserted := 0
	for _, ep := range report.Episodes {
		var n int
		_ = s.db.QueryRow(`SELECT count(*) FROM memory_edges WHERE (source_id=? OR target_id=?) AND type=?`, ep.ID, ep.ID, EdgeTaughtFrom).Scan(&n)
		_ = n
		inserted++
	}
	report.Inserted = len(ids)
	sample := ids
	if len(sample) > 3 {
		sample = sample[:3]
	}
	for i, id := range sample {
		cue := firstLine(report.Episodes[i].Text, 80)
		if cue == "" {
			cue = report.Episodes[i].Title
		}
		hits, err := s.Search(SearchFilter{Query: cue, Kind: KindTeaching, Limit: 5})
		report.RecallTested++
		if err != nil {
			continue
		}
		for _, h := range hits {
			if h.ID == id {
				report.RecallVerified++
				break
			}
		}
	}
	return report, nil
}

func chunkTeach(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	words := strings.Fields(text)
	if EstimateTokens(text) <= teachChunkTokens {
		return []string{text}
	}
	var chunks []string
	step := teachChunkTokens - teachOverlapTok
	if step < 32 {
		step = 32
	}
	for i := 0; i < len(words); {
		end := i + teachChunkTokens
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		if len(chunk) >= 12 {
			chunks = append(chunks, chunk)
		}
		if end == len(words) {
			break
		}
		i += step
	}
	return chunks
}

func extractTeachText(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return extractPDFText(data)
	case ".docx", ".pptx", ".xlsx", ".epub":
		return extractZipXML(data)
	default:
		if !utf8.Valid(data) {
			return extractPDFText(data)
		}
		return string(data)
	}
}

var xmlTextRE = regexp.MustCompile(`>([^<]{2,})<`)

func extractZipXML(data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return string(data)
	}
	var b strings.Builder
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".html") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(rc)
		_ = rc.Close()
		for _, m := range xmlTextRE.FindAllSubmatch(raw, 4000) {
			s := strings.TrimSpace(string(m[1]))
			if printable(s) {
				b.WriteString(s)
				b.WriteByte(' ')
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var pdfStringRE = regexp.MustCompile(`\((?:\\.|[^\\)]){2,}\)`)

func extractPDFText(data []byte) string {
	matches := pdfStringRE.FindAll(data, 4000)
	var b strings.Builder
	for _, m := range matches {
		s := string(m)
		s = strings.TrimPrefix(s, "(")
		s = strings.TrimSuffix(s, ")")
		s = strings.ReplaceAll(s, `\(`, "(")
		s = strings.ReplaceAll(s, `\)`, ")")
		s = strings.ReplaceAll(s, `\n`, "\n")
		s = strings.ReplaceAll(s, `\r`, "")
		if !printable(s) {
			continue
		}
		b.WriteString(s)
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

func printable(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		} else if r < 32 && r != '\n' && r != '\t' {
			return false
		}
	}
	return letters >= 3
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [12]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
