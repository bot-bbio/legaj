package resume

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// wordML is the WordprocessingML namespace of word/document.xml.
const wordML = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// Zip-bomb limits: a resume's document.xml is a few hundred KB at most.
const (
	maxDocumentXML = 20 << 20
	maxDOCXText    = 200_000
)

func documentXML(data []byte) (*zip.File, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			return f, nil
		}
	}
	return nil, errors.New("not a Word document: no word/document.xml")
}

func isDOCX(data []byte) bool {
	_, err := documentXML(data)
	return err == nil
}

// DOCXText extracts a .docx file's text: runs joined, a newline per
// paragraph and line break, tabs kept. It reads XML with Go's decoder,
// which doesn't expand custom entities, under a size cap.
func DOCXText(data []byte) (string, error) {
	f, err := documentXML(data)
	if err != nil {
		return "", err
	}
	if f.UncompressedSize64 > maxDocumentXML {
		return "", errors.New("word/document.xml is too large")
	}
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, maxDocumentXML))
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != wordML {
				continue
			}
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			if t.Name.Space != wordML {
				continue
			}
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
		if sb.Len() > maxDOCXText {
			return "", errors.New("the document has too much text for a resume")
		}
	}
	return sb.String(), nil
}
