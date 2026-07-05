package workbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"im-go/internal/auth"
)

const defaultKnowledgeUploadRoot = "data/uploads/knowledge"

var (
	ErrKnowledgeDocWriteStoreUnavailable = errors.New("workbench knowledge doc write store is unavailable")
	ErrKnowledgeDocNotFound              = errors.New("document not found")
	ErrKnowledgeDocFileRequired          = errors.New("file is required")
	ErrKnowledgeSearchQueryRequired      = errors.New("query is required")
	ErrKnowledgeSearchQRequired          = errors.New("q is required")
	ErrKnowledgeDialogueQuestionRequired = errors.New("question is required")
)

var (
	knowledgeAllowedExtensions = map[string]struct{}{
		".doc":  {},
		".docx": {},
		".md":   {},
		".pdf":  {},
		".txt":  {},
	}
	knowledgeWordPattern           = regexp.MustCompile(`[a-z0-9_]{2,}`)
	knowledgeCodePattern           = regexp.MustCompile("(?is)```(?:text)?\\s*([\\s\\S]*?)```")
	knowledgeSectionPattern        = regexp.MustCompile(`(?m)^###\s*(.+?)\s*$`)
	knowledgeStandardAnswerPattern = regexp.MustCompile("(?is)standard_answer`?:?\\s*[\\s\\S]*?```(?:text)?\\s*([\\s\\S]*?)```")
)

// KnowledgeDocUnsupportedFileTypeError mirrors the Python validation detail.
type KnowledgeDocUnsupportedFileTypeError struct {
	Extension string
}

// Error returns the legacy unsupported-file validation message.
func (err KnowledgeDocUnsupportedFileTypeError) Error() string {
	return fmt.Sprintf("不支持的文件类型: %s，允许的类型: .doc, .docx, .md, .pdf, .txt", err.Extension)
}

// KnowledgeDocAddCommand registers a newly uploaded knowledge document.
type KnowledgeDocAddCommand struct {
	Filename  string
	FilePath  string
	SizeBytes int
}

// KnowledgeDocUpdateCommand replaces an existing uploaded document.
type KnowledgeDocUpdateCommand struct {
	DocID     string
	Filename  string
	FilePath  string
	SizeBytes int
	Status    string
}

// KnowledgeDocUploadRequest carries multipart upload bytes after HTTP parsing.
type KnowledgeDocUploadRequest struct {
	Filename string
	Content  []byte
	Session  auth.Session
}

// NewKnowledgeDocUploadRequest normalizes an upload request.
func NewKnowledgeDocUploadRequest(filename string, content []byte, session auth.Session) KnowledgeDocUploadRequest {
	return KnowledgeDocUploadRequest{Filename: strings.TrimSpace(filename), Content: content, Session: session}
}

// KnowledgeDocUpdateRequest carries replacement upload bytes.
type KnowledgeDocUpdateRequest struct {
	DocID    string
	Filename string
	Content  []byte
	Session  auth.Session
}

// NewKnowledgeDocUpdateRequest normalizes a replacement request.
func NewKnowledgeDocUpdateRequest(docID string, filename string, content []byte, session auth.Session) KnowledgeDocUpdateRequest {
	return KnowledgeDocUpdateRequest{DocID: strings.TrimSpace(docID), Filename: strings.TrimSpace(filename), Content: content, Session: session}
}

// KnowledgeDocDeleteRequest carries a document id to delete.
type KnowledgeDocDeleteRequest struct {
	DocID   string
	Session auth.Session
}

// NewKnowledgeDocDeleteRequest normalizes a delete request.
func NewKnowledgeDocDeleteRequest(docID string, session auth.Session) KnowledgeDocDeleteRequest {
	return KnowledgeDocDeleteRequest{DocID: strings.TrimSpace(docID), Session: session}
}

// KnowledgeDocReindexRequest carries a document id to reindex.
type KnowledgeDocReindexRequest struct {
	DocID   string
	Session auth.Session
}

// NewKnowledgeDocReindexRequest normalizes a reindex request.
func NewKnowledgeDocReindexRequest(docID string, session auth.Session) KnowledgeDocReindexRequest {
	return KnowledgeDocReindexRequest{DocID: strings.TrimSpace(docID), Session: session}
}

// KnowledgeSearchRequest carries admin/cs knowledge search input.
type KnowledgeSearchRequest struct {
	Query         string
	MissingDetail string
	Session       auth.Session
}

// NewKnowledgeSearchRequest normalizes a knowledge search request.
func NewKnowledgeSearchRequest(query string, missingDetail string, session auth.Session) KnowledgeSearchRequest {
	return KnowledgeSearchRequest{Query: strings.TrimSpace(query), MissingDetail: strings.TrimSpace(missingDetail), Session: session}
}

// KnowledgeDialogueBody is the JSON input for the legacy knowledge dialogue test.
type KnowledgeDialogueBody struct {
	Question string `json:"question"`
	Prompt   string `json:"prompt"`
	TopK     int    `json:"top_k"`
}

// KnowledgeDialogueRequest carries an admin-scoped knowledge dialogue probe.
type KnowledgeDialogueRequest struct {
	Question string
	TopK     int
	Session  auth.Session
}

// NewKnowledgeDialogueRequest normalizes the knowledge dialogue request boundary.
func NewKnowledgeDialogueRequest(body KnowledgeDialogueBody, session auth.Session) KnowledgeDialogueRequest {
	question := strings.TrimSpace(body.Question)
	if question == "" {
		question = strings.TrimSpace(body.Prompt)
	}
	topK := body.TopK
	if topK <= 0 {
		topK = 3
	}
	return KnowledgeDialogueRequest{Question: question, TopK: topK, Session: session}
}

// UploadKnowledgeDoc saves an uploaded file and registers its metadata.
func (service Service) UploadKnowledgeDoc(ctx context.Context, request KnowledgeDocUploadRequest) (Payload, error) {
	if service.KnowledgeDocWriteStore == nil {
		return nil, ErrKnowledgeDocWriteStoreUnavailable
	}
	filename := request.Filename
	if filename == "" {
		filename = "unknown.txt"
	}
	if err := validateKnowledgeFilename(filename); err != nil {
		return nil, err
	}
	filePath, err := service.writeKnowledgeFile(filename, request.Content)
	if err != nil {
		return nil, err
	}
	doc, err := service.KnowledgeDocWriteStore.AddKnowledgeDoc(ctx, KnowledgeDocAddCommand{
		Filename:  filename,
		FilePath:  filePath,
		SizeBytes: len(request.Content),
	})
	if err != nil {
		return nil, err
	}
	return Payload{"success": true, "document": knowledgeDocDocumentPayload(doc)}, nil
}

// UpdateKnowledgeDoc replaces an existing knowledge document file.
func (service Service) UpdateKnowledgeDoc(ctx context.Context, request KnowledgeDocUpdateRequest) (Payload, error) {
	if service.KnowledgeDocWriteStore == nil {
		return nil, ErrKnowledgeDocWriteStoreUnavailable
	}
	original, ok, err := service.KnowledgeDocWriteStore.GetKnowledgeDoc(ctx, request.DocID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	filename := request.Filename
	if filename == "" {
		filename = strings.TrimSpace(original.Filename)
	}
	if filename == "" {
		filename = "unknown.txt"
	}
	if err := validateKnowledgeFilename(filename); err != nil {
		return nil, err
	}
	filePath, err := service.writeKnowledgeFile(filename, request.Content)
	if err != nil {
		return nil, err
	}
	removeKnowledgeFile(original.FilePath, filePath)
	doc, ok, err := service.KnowledgeDocWriteStore.UpdateKnowledgeDoc(ctx, KnowledgeDocUpdateCommand{
		DocID:     request.DocID,
		Filename:  filename,
		FilePath:  filePath,
		SizeBytes: len(request.Content),
		Status:    "pending",
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	return Payload{"success": true, "document": knowledgeDocDocumentPayload(doc)}, nil
}

// DeleteKnowledgeDoc deletes a document metadata row and best-effort removes its file.
func (service Service) DeleteKnowledgeDoc(ctx context.Context, request KnowledgeDocDeleteRequest) (Payload, error) {
	if service.KnowledgeDocWriteStore == nil {
		return nil, ErrKnowledgeDocWriteStoreUnavailable
	}
	doc, ok, err := service.KnowledgeDocWriteStore.GetKnowledgeDoc(ctx, request.DocID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	deleted, err := service.KnowledgeDocWriteStore.DeleteKnowledgeDoc(ctx, request.DocID)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, ErrKnowledgeDocNotFound
	}
	removeKnowledgeFile(doc.FilePath, "")
	return Payload{"success": true}, nil
}

// ReindexKnowledgeDoc mirrors the Python placeholder indexing transition.
func (service Service) ReindexKnowledgeDoc(ctx context.Context, request KnowledgeDocReindexRequest) (Payload, error) {
	if service.KnowledgeDocWriteStore == nil {
		return nil, ErrKnowledgeDocWriteStoreUnavailable
	}
	if _, ok, err := service.KnowledgeDocWriteStore.GetKnowledgeDoc(ctx, request.DocID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	if ok, err := service.KnowledgeDocWriteStore.UpdateKnowledgeDocStatus(ctx, request.DocID, "indexing"); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	if ok, err := service.KnowledgeDocWriteStore.UpdateKnowledgeDocStatus(ctx, request.DocID, "indexed"); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	doc, ok, err := service.KnowledgeDocWriteStore.GetKnowledgeDoc(ctx, request.DocID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrKnowledgeDocNotFound
	}
	return Payload{"success": true, "document": knowledgeDocDocumentPayload(doc)}, nil
}

// SearchKnowledge runs the lightweight legacy file search over uploaded docs.
func (service Service) SearchKnowledge(ctx context.Context, request KnowledgeSearchRequest) (Payload, error) {
	if request.Query == "" {
		if request.MissingDetail == "q" {
			return nil, ErrKnowledgeSearchQRequired
		}
		return nil, ErrKnowledgeSearchQueryRequired
	}
	if service.KnowledgeDocStore == nil {
		return nil, ErrKnowledgeDocStoreUnavailable
	}
	docs, err := service.KnowledgeDocStore.ListKnowledgeDocs(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"results": searchKnowledgeDocs(request.Query, docs)}, nil
}

// KnowledgeDialogue mirrors Python KnowledgeService.test_dialogue: it first
// ranks structured Markdown QA entries, then falls back to snippet search.
func (service Service) KnowledgeDialogue(ctx context.Context, request KnowledgeDialogueRequest) (Payload, error) {
	if request.Question == "" {
		return nil, ErrKnowledgeDialogueQuestionRequired
	}
	if service.KnowledgeDocStore == nil {
		return nil, ErrKnowledgeDocStoreUnavailable
	}
	docs, err := service.KnowledgeDocStore.ListKnowledgeDocs(ctx)
	if err != nil {
		return nil, err
	}
	topK := request.TopK
	if topK <= 0 {
		topK = 3
	}
	ranked := rankKnowledgeQAEntries(request.Question, collectKnowledgeQAEntries(docs))
	if len(ranked) > 0 {
		best := ranked[0]
		return Payload{
			"reply":            best.Answer,
			"matched_question": best.Question,
			"source":           best.Source,
			"confidence":       roundKnowledgeScore(best.Score),
			"mode":             "knowledge_qa",
			"candidates":       knowledgeQACandidatePayload(ranked, topK),
		}, nil
	}
	fallback := searchKnowledgeDocs(request.Question, docs)
	if len(fallback) > 0 {
		first := fallback[0]
		reply := formatKnowledgeSnippetReply(stringFromAny(first["content"]))
		if reply == "" {
			reply = "我找到了一些相关资料，建议转人工进一步确认。"
		}
		return Payload{
			"reply":            reply,
			"matched_question": "",
			"source":           stringFromAny(first["source"]),
			"confidence":       0.2,
			"mode":             "snippet_fallback",
			"candidates":       limitProjectionRows(fallback, topK),
		}, nil
	}
	return Payload{
		"reply":            "暂时没有在知识库找到明确答案，建议转人工消息端进一步确认。",
		"matched_question": "",
		"source":           "",
		"confidence":       0.0,
		"mode":             "no_match",
		"candidates":       []ProjectionRow{},
	}, nil
}

func validateKnowledgeFilename(filename string) error {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if _, ok := knowledgeAllowedExtensions[ext]; !ok {
		return KnowledgeDocUnsupportedFileTypeError{Extension: ext}
	}
	return nil
}

func (service Service) writeKnowledgeFile(filename string, content []byte) (string, error) {
	root := strings.TrimSpace(service.KnowledgeUploadRoot)
	if root == "" {
		root = defaultKnowledgeUploadRoot
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	safeName := fmt.Sprintf("%s-%s%s", service.knowledgeNow().UTC().Format("20060102150405"), service.nextKnowledgeFileToken(), ext)
	dest := filepath.Join(root, safeName)
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

func (service Service) knowledgeNow() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service Service) nextKnowledgeFileToken() string {
	if service.NextKnowledgeFileToken != nil {
		if value := strings.TrimSpace(service.NextKnowledgeFileToken()); value != "" {
			if len(value) > 8 {
				return value[:8]
			}
			return value
		}
	}
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())[:8]
	}
	return hex.EncodeToString(random[:])
}

func removeKnowledgeFile(path string, newPath string) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" || cleaned == strings.TrimSpace(newPath) {
		return
	}
	_ = os.Remove(cleaned)
}

func knowledgeDocDocumentPayload(doc KnowledgeDocRecord) ProjectionRow {
	payload := knowledgeDocPayload([]KnowledgeDocRecord{doc})
	if len(payload) == 0 {
		return ProjectionRow{}
	}
	return payload[0]
}

func searchKnowledgeDocs(query string, docs []KnowledgeDocRecord) []ProjectionRow {
	results := make([]ProjectionRow, 0)
	keyword := strings.ToLower(strings.TrimSpace(query))
	queryTerms := buildKnowledgeTerms(query)
	for _, doc := range docs {
		path := strings.TrimSpace(doc.FilePath)
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(raw)
		lowerText := strings.ToLower(text)
		hitByteIndex := strings.Index(lowerText, keyword)
		matchedTerm := keyword
		score := 0.0
		if hitByteIndex >= 0 {
			score = 1.0
		} else {
			docTerms := buildKnowledgeTerms(text)
			overlap := termOverlap(queryTerms, docTerms)
			if overlap <= 0 {
				continue
			}
			score = float64(overlap) / math.Max(float64(len(queryTerms)), 1)
			candidates := matchedKnowledgeTerms(queryTerms, lowerText)
			sort.Slice(candidates, func(i int, j int) bool {
				left := candidates[i]
				right := candidates[j]
				if runeLen(left) != runeLen(right) {
					return runeLen(left) > runeLen(right)
				}
				if strings.Count(lowerText, left) != strings.Count(lowerText, right) {
					return strings.Count(lowerText, left) < strings.Count(lowerText, right)
				}
				return strings.Index(lowerText, left) < strings.Index(lowerText, right)
			})
			if len(candidates) > 0 {
				matchedTerm = candidates[0]
				hitByteIndex = strings.Index(lowerText, matchedTerm)
			}
			if hitByteIndex < 0 {
				hitByteIndex = 0
			}
		}
		hitRuneIndex := byteToRuneIndex(lowerText, hitByteIndex)
		snippet := extractKnowledgeSnippet(text, queryTerms, hitRuneIndex, maxInt(runeLen(keyword), runeLen(matchedTerm)))
		results = append(results, ProjectionRow{
			"source":  strings.TrimSpace(doc.Filename),
			"doc_id":  strings.TrimSpace(doc.DocID),
			"content": snippet,
			"score":   roundKnowledgeScore(score),
		})
	}
	sort.SliceStable(results, func(i int, j int) bool {
		return floatFromPayload(results[i]["score"]) > floatFromPayload(results[j]["score"])
	})
	return results
}

type knowledgeQAEntry struct {
	Question string
	Answer   string
	Source   string
	Score    float64
}

func collectKnowledgeQAEntries(docs []KnowledgeDocRecord) []knowledgeQAEntry {
	entries := make([]knowledgeQAEntry, 0)
	for _, doc := range docs {
		if strings.TrimSpace(doc.Status) != "indexed" {
			continue
		}
		raw, err := os.ReadFile(strings.TrimSpace(doc.FilePath))
		if err != nil {
			continue
		}
		for _, entry := range extractKnowledgeMarkdownQA(string(raw)) {
			entry.Source = strings.TrimSpace(doc.Filename)
			if entry.Question != "" && entry.Answer != "" {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func rankKnowledgeQAEntries(question string, entries []knowledgeQAEntry) []knowledgeQAEntry {
	queryTerms := buildKnowledgeTerms(question)
	if len(queryTerms) == 0 {
		return nil
	}
	ranked := make([]knowledgeQAEntry, 0, len(entries))
	for _, entry := range entries {
		targetTerms := buildKnowledgeTerms(entry.Question)
		if len(targetTerms) == 0 {
			continue
		}
		score := float64(termOverlap(queryTerms, targetTerms)) / math.Max(float64(len(queryTerms)), 1)
		if score <= 0 {
			continue
		}
		entry.Score = score
		ranked = append(ranked, entry)
	}
	sort.SliceStable(ranked, func(i int, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	return ranked
}

func extractKnowledgeMarkdownQA(text string) []knowledgeQAEntry {
	payload := make([]knowledgeQAEntry, 0)
	sections := knowledgeSectionPattern.FindAllStringSubmatchIndex(text, -1)
	for index, section := range sections {
		if len(section) < 4 || section[2] < 0 || section[3] < 0 {
			continue
		}
		bodyStart := section[1]
		bodyEnd := len(text)
		if index+1 < len(sections) {
			bodyEnd = sections[index+1][0]
		}
		heading := strings.TrimSpace(text[section[2]:section[3]])
		body := text[bodyStart:bodyEnd]
		answerMatch := knowledgeStandardAnswerPattern.FindStringSubmatch(body)
		if len(answerMatch) < 2 {
			continue
		}
		answer := strings.TrimSpace(answerMatch[1])
		if heading != "" && answer != "" {
			payload = append(payload, knowledgeQAEntry{Question: heading, Answer: answer})
		}
	}
	payload = append(payload, extractKnowledgeLineQA(text)...)
	return uniqueKnowledgeQAEntries(payload)
}

func extractKnowledgeLineQA(text string) []knowledgeQAEntry {
	entries := make([]knowledgeQAEntry, 0)
	question := ""
	answerLines := make([]string, 0)
	answerMode := false
	flush := func() {
		answer := strings.TrimSpace(strings.Join(answerLines, "\n"))
		if question != "" && answer != "" {
			entries = append(entries, knowledgeQAEntry{Question: question, Answer: answer})
		}
		question = ""
		answerLines = answerLines[:0]
		answerMode = false
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			if answerMode {
				answerLines = append(answerLines, "")
			}
			continue
		}
		if label, value, ok := splitKnowledgeLabel(line); ok {
			switch label {
			case "q", "question", "问题":
				flush()
				question = value
			case "a", "answer", "回答", "答案", "standard_answer":
				if question != "" {
					answerLines = answerLines[:0]
					if value != "" {
						answerLines = append(answerLines, value)
					}
					answerMode = true
				}
			}
			continue
		}
		if answerMode {
			answerLines = append(answerLines, line)
		}
	}
	flush()
	return entries
}

func splitKnowledgeLabel(line string) (string, string, bool) {
	cleaned := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
	separator := strings.Index(cleaned, ":")
	fullSeparator := strings.Index(cleaned, "：")
	separatorSize := len(":")
	if separator < 0 || (fullSeparator >= 0 && fullSeparator < separator) {
		separator = fullSeparator
		separatorSize = len("：")
	}
	if separator < 0 {
		return "", "", false
	}
	label := strings.ToLower(strings.TrimSpace(cleaned[:separator]))
	value := strings.TrimSpace(cleaned[separator+separatorSize:])
	if label == "" {
		return "", "", false
	}
	switch label {
	case "q", "question", "问题", "a", "answer", "回答", "答案", "standard_answer":
		return label, value, true
	default:
		return "", "", false
	}
}

func uniqueKnowledgeQAEntries(entries []knowledgeQAEntry) []knowledgeQAEntry {
	unique := make([]knowledgeQAEntry, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		key := entry.Question + "\x00" + entry.Answer
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, entry)
	}
	return unique
}

func knowledgeQACandidatePayload(entries []knowledgeQAEntry, limit int) []ProjectionRow {
	if limit <= 0 {
		limit = 1
	}
	if limit > len(entries) {
		limit = len(entries)
	}
	payload := make([]ProjectionRow, 0, limit)
	for _, entry := range entries[:limit] {
		payload = append(payload, ProjectionRow{
			"question":   entry.Question,
			"answer":     entry.Answer,
			"source":     entry.Source,
			"confidence": roundKnowledgeScore(entry.Score),
		})
	}
	return payload
}

func limitProjectionRows(rows []ProjectionRow, limit int) []ProjectionRow {
	if limit <= 0 {
		limit = 1
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	return append([]ProjectionRow(nil), rows[:limit]...)
}

func formatKnowledgeSnippetReply(snippet string) string {
	text := strings.TrimSpace(snippet)
	if text == "" {
		return ""
	}
	codeBlocks := knowledgeCodePattern.FindAllStringSubmatch(text, -1)
	for _, block := range codeBlocks {
		if len(block) < 2 {
			continue
		}
		lines := make([]string, 0)
		for _, raw := range strings.Split(block[1], "\n") {
			line := strings.TrimSpace(raw)
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			return strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	lines := make([]string, 0)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "- 特征") || strings.HasPrefix(line, "- 目标") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func buildKnowledgeTerms(text string) map[string]struct{} {
	normalized := strings.ToLower(strings.TrimSpace(text))
	terms := map[string]struct{}{}
	if normalized == "" {
		return terms
	}
	for _, word := range knowledgeWordPattern.FindAllString(normalized, -1) {
		terms[word] = struct{}{}
	}
	zhChars := make([]rune, 0)
	for _, char := range normalized {
		if char >= '\u4e00' && char <= '\u9fff' {
			zhChars = append(zhChars, char)
			terms[string(char)] = struct{}{}
		}
	}
	for index := 0; index+1 < len(zhChars); index++ {
		terms[string([]rune{zhChars[index], zhChars[index+1]})] = struct{}{}
	}
	return terms
}

func termOverlap(left map[string]struct{}, right map[string]struct{}) int {
	overlap := 0
	for term := range left {
		if _, ok := right[term]; ok {
			overlap++
		}
	}
	return overlap
}

func matchedKnowledgeTerms(queryTerms map[string]struct{}, lowerText string) []string {
	candidates := make([]string, 0)
	for term := range queryTerms {
		if term != "" && strings.Contains(lowerText, term) {
			candidates = append(candidates, term)
		}
	}
	return candidates
}

func extractKnowledgeSnippet(text string, queryTerms map[string]struct{}, hitIndex int, hitLength int) string {
	if text == "" {
		return ""
	}
	bestBlock := ""
	bestOverlap := -1
	bestDistance := math.MaxInt
	for _, match := range knowledgeCodePattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 4 || match[2] < 0 || match[3] < 0 {
			continue
		}
		start := byteToRuneIndex(text, match[0])
		end := byteToRuneIndex(text, match[1])
		if hitIndex < start-260 || hitIndex > end+260 {
			continue
		}
		block := strings.TrimSpace(text[match[2]:match[3]])
		if block == "" {
			continue
		}
		overlap := termOverlap(queryTerms, buildKnowledgeTerms(block))
		distance := minInt(absInt(hitIndex-start), absInt(hitIndex-end))
		if overlap > bestOverlap || (overlap == bestOverlap && distance < bestDistance) {
			bestBlock = block
			bestOverlap = overlap
			bestDistance = distance
		}
	}
	if bestBlock != "" {
		return bestBlock
	}
	sourceRunes := []rune(text)
	start := maxInt(0, hitIndex-120)
	end := minInt(len(sourceRunes), hitIndex+maxInt(1, hitLength)+280)
	return strings.TrimSpace(string(sourceRunes[start:end]))
}

func byteToRuneIndex(text string, byteIndex int) int {
	if byteIndex <= 0 {
		return 0
	}
	if byteIndex >= len(text) {
		return len([]rune(text))
	}
	return len([]rune(text[:byteIndex]))
}

func runeLen(text string) int {
	return len([]rune(text))
}

func roundKnowledgeScore(score float64) float64 {
	return math.Round(score*10000) / 10000
}

func floatFromPayload(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
