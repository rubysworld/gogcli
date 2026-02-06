package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDocsService = googleapi.NewDocs

type DocsCmd struct {
	Export DocsExportCmd `cmd:"" name:"export" help:"Export a Google Doc (pdf|docx|txt)"`
	Info   DocsInfoCmd   `cmd:"" name:"info" help:"Get Google Doc metadata"`
	Create DocsCreateCmd `cmd:"" name:"create" help:"Create a Google Doc"`
	Copy   DocsCopyCmd   `cmd:"" name:"copy" help:"Copy a Google Doc"`
	Cat    DocsCatCmd    `cmd:"" name:"cat" help:"Print a Google Doc as plain text"`
	Update DocsUpdateCmd `cmd:"" name:"update" help:"Update a Google Doc content"`
	Append DocsAppendCmd `cmd:"" name:"append" help:"Append content to a Google Doc"`
}

type DocsExportCmd struct {
	DocID  string         `arg:"" name:"docId" help:"Doc ID"`
	Output OutputPathFlag `embed:""`
	Format string         `name:"format" help:"Export format: pdf|docx|txt" default:"pdf"`
}

func (c *DocsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		ArgName:       "docId",
		ExpectedMime:  "application/vnd.google-apps.document",
		KindLabel:     "Google Doc",
		DefaultFormat: "pdf",
	}, c.DocID, c.Output.Path, c.Format)
}

type DocsInfoCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsInfoCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		Fields("documentId,title,revisionId").
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	file := map[string]any{
		"id":       doc.DocumentId,
		"name":     doc.Title,
		"mimeType": driveMimeGoogleDoc,
	}
	if link := docsWebViewLink(doc.DocumentId); link != "" {
		file["webViewLink"] = link
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			strFile:    file,
			"document": doc,
		})
	}

	u.Out().Printf("id\t%s", doc.DocumentId)
	u.Out().Printf("name\t%s", doc.Title)
	u.Out().Printf("mime\t%s", driveMimeGoogleDoc)
	if link := docsWebViewLink(doc.DocumentId); link != "" {
		u.Out().Printf("link\t%s", link)
	}
	if doc.RevisionId != "" {
		u.Out().Printf("revision\t%s", doc.RevisionId)
	}
	return nil
}

type DocsCreateCmd struct {
	Title       string `arg:"" name:"title" help:"Doc title"`
	Parent      string `name:"parent" help:"Destination folder ID"`
	Content     string `name:"content" help:"Initial text content"`
	ContentFile string `name:"content-file" help:"Read initial content from file"`
}

func (c *DocsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	// Get content from flag or file
	content, err := resolveContent(c.Content, c.ContentFile)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	f := &drive.File{
		Name:     title,
		MimeType: "application/vnd.google-apps.document",
	}
	parent := strings.TrimSpace(c.Parent)
	if parent != "" {
		f.Parents = []string{parent}
	}

	created, err := svc.Files.Create(f).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("create failed")
	}

	// If content provided, insert it using Docs API
	if content != "" {
		docsSvc, err := newDocsService(ctx, account)
		if err != nil {
			return fmt.Errorf("docs service: %w", err)
		}

		req := &docs.BatchUpdateDocumentRequest{
			Requests: []*docs.Request{
				{
					InsertText: &docs.InsertTextRequest{
						Text: content,
						Location: &docs.Location{
							Index: 1, // Insert at beginning (after the implicit newline)
						},
					},
				},
			},
		}

		_, err = docsSvc.Documents.BatchUpdate(created.Id, req).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("insert content: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{strFile: created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}

type DocsCopyCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Title  string `arg:"" name:"title" help:"New title"`
	Parent string `name:"parent" help:"Destination folder ID"`
}

func (c *DocsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		ArgName:      "docId",
		ExpectedMime: "application/vnd.google-apps.document",
		KindLabel:    "Google Doc",
	}, c.DocID, c.Title, c.Parent)
}

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
}

func (c *DocsCatCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	text := docsPlainText(doc, c.MaxBytes)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"text": text})
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

type DocsUpdateCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	Content     string `name:"content" help:"New text content"`
	ContentFile string `name:"content-file" help:"Read content from file"`
	ReplaceAll  bool   `name:"replace-all" help:"Replace all existing content"`
	InsertAt    int64  `name:"insert-at" help:"Insert at specific index (1-based)" default:"1"`
}

func (c *DocsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	content, err := resolveContent(c.Content, c.ContentFile)
	if err != nil {
		return err
	}
	if content == "" {
		return usage("no content provided (use --content or --content-file)")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	var requests []*docs.Request

	if c.ReplaceAll {
		// Get the document to find its length
		doc, err := svc.Documents.Get(id).Context(ctx).Do()
		if err != nil {
			if isDocsNotFound(err) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
			}
			return err
		}

		// Calculate end index (Body.Content has structural elements, last one's EndIndex - 1)
		endIndex := getDocEndIndex(doc)
		if endIndex > 1 {
			// Delete existing content (from index 1 to end-1, preserving trailing newline)
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
						EndIndex:   endIndex,
					},
				},
			})
		}
	}

	// Insert new content
	insertIndex := c.InsertAt
	if insertIndex < 1 {
		insertIndex = 1
	}
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Text: content,
			Location: &docs.Location{
				Index: insertIndex,
			},
		},
	})

	req := &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"updated":    true,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("updated\ttrue")
	if link := docsWebViewLink(resp.DocumentId); link != "" {
		u.Out().Printf("link\t%s", link)
	}
	return nil
}

type DocsAppendCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	Content     string `name:"content" help:"Text content to append"`
	ContentFile string `name:"content-file" help:"Read content from file"`
	Newline     bool   `name:"newline" help:"Add newline before appending" default:"true"`
}

func (c *DocsAppendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	content, err := resolveContent(c.Content, c.ContentFile)
	if err != nil {
		return err
	}
	if content == "" {
		return usage("no content provided (use --content or --content-file)")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	// Get the document to find its end position
	doc, err := svc.Documents.Get(id).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}

	// Get end index for insertion
	endIndex := getDocEndIndex(doc)

	// Prepend newline if requested and doc has content
	textToInsert := content
	if c.Newline && endIndex > 1 {
		textToInsert = "\n" + content
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				InsertText: &docs.InsertTextRequest{
					Text: textToInsert,
					Location: &docs.Location{
						Index: endIndex,
					},
				},
			},
		},
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("append failed: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"appended":   true,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("appended\ttrue")
	if link := docsWebViewLink(resp.DocumentId); link != "" {
		u.Out().Printf("link\t%s", link)
	}
	return nil
}

// resolveContent returns content from --content flag or reads from --content-file
func resolveContent(content, contentFile string) (string, error) {
	if content != "" && contentFile != "" {
		return "", errors.New("cannot use both --content and --content-file")
	}
	if contentFile != "" {
		data, err := os.ReadFile(contentFile)
		if err != nil {
			return "", fmt.Errorf("read content file: %w", err)
		}
		return string(data), nil
	}
	return content, nil
}

// getDocEndIndex returns the index position at the end of the document body
func getDocEndIndex(doc *docs.Document) int64 {
	if doc == nil || doc.Body == nil || len(doc.Body.Content) == 0 {
		return 1
	}
	// The last element's EndIndex points to the position after all content
	last := doc.Body.Content[len(doc.Body.Content)-1]
	if last.EndIndex > 1 {
		return last.EndIndex - 1 // -1 to stay before the implicit trailing newline
	}
	return 1
}

func docsWebViewLink(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "https://docs.google.com/document/d/" + id + "/edit"
}

func docsPlainText(doc *docs.Document, maxBytes int64) string {
	if doc == nil || doc.Body == nil {
		return ""
	}

	var buf bytes.Buffer
	for _, el := range doc.Body.Content {
		if !appendDocsElementText(&buf, maxBytes, el) {
			break
		}
	}

	return buf.String()
}

func appendDocsElementText(buf *bytes.Buffer, maxBytes int64, el *docs.StructuralElement) bool {
	if el == nil {
		return true
	}

	switch {
	case el.Paragraph != nil:
		for _, p := range el.Paragraph.Elements {
			if p.TextRun == nil {
				continue
			}
			if !appendLimited(buf, maxBytes, p.TextRun.Content) {
				return false
			}
		}
	case el.Table != nil:
		for rowIdx, row := range el.Table.TableRows {
			if rowIdx > 0 {
				if !appendLimited(buf, maxBytes, "\n") {
					return false
				}
			}
			for cellIdx, cell := range row.TableCells {
				if cellIdx > 0 {
					if !appendLimited(buf, maxBytes, "\t") {
						return false
					}
				}
				for _, content := range cell.Content {
					if !appendDocsElementText(buf, maxBytes, content) {
						return false
					}
				}
			}
		}
	case el.TableOfContents != nil:
		for _, content := range el.TableOfContents.Content {
			if !appendDocsElementText(buf, maxBytes, content) {
				return false
			}
		}
	}

	return true
}

func appendLimited(buf *bytes.Buffer, maxBytes int64, s string) bool {
	if maxBytes <= 0 {
		_, _ = buf.WriteString(s)
		return true
	}

	remaining := int(maxBytes) - buf.Len()
	if remaining <= 0 {
		return false
	}
	if len(s) > remaining {
		_, _ = buf.WriteString(s[:remaining])
		return false
	}
	_, _ = buf.WriteString(s)
	return true
}

func isDocsNotFound(err error) bool {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusNotFound
}
