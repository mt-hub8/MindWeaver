package pdfqualification_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

// The fixtures in this file are generated from short, reviewable object lists.
// Qualification never downloads or checks in opaque PDF corpus binaries.

func generateFlateToUnicodePDF(spec pdfTextLayerSpec) ([]byte, error) {
	if len(spec.Pages) != 1 {
		return nil, fmt.Errorf("Flate qualification PDF requires one page")
	}
	var content strings.Builder
	for index, line := range spec.Pages[0].Lines {
		fmt.Fprintf(&content, "BT\n/F0 11 Tf\n72 %d Td\n<%s> Tj\nET\n", 790-index*18, utf16BEHex(line))
	}
	mediaBox := fmt.Sprintf("[%d %d %d %d]", spec.MediaBox[0], spec.MediaBox[1], spec.MediaBox[2], spec.MediaBox[3])
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox %s /Resources << /Font << /F0 5 0 R >> >> /Contents 4 0 R >>", mediaBox),
		flatePDFStream([]byte(content.String())),
		"<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
		qualificationCIDFontObject,
		flatePDFStream([]byte(toUnicodeCMap(spec.Pages[0].Lines))),
	}
	return serializePDF(objects, ""), nil
}

func generateMultiPageUniGBPDF(texts []string) []byte {
	pageCount := len(texts)
	fontObject := 3 + pageCount*2
	cidFontObject := fontObject + 1
	objects := make([]string, cidFontObject)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	var kids strings.Builder
	for index := range texts {
		fmt.Fprintf(&kids, "%d 0 R ", 3+index)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pageCount)
	for index, text := range texts {
		pageObject := 3 + index
		contentObject := 3 + pageCount + index
		objects[pageObject-1] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F0 %d 0 R >> >> /Contents %d 0 R >>", fontObject, contentObject)
		content := fmt.Sprintf("BT /F0 11 Tf 72 720 Td <%s> Tj ET", utf16BEHex(text))
		objects[contentObject-1] = pdfStream([]byte(content))
	}
	objects[fontObject-1] = fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [%d 0 R] >>", cidFontObject)
	objects[cidFontObject-1] = qualificationCIDFontObject
	return serializePDF(objects, "")
}

func generateManyPagePDF(pageCount int) []byte {
	contentObject := pageCount + 3
	fontObject := contentObject + 1
	objects := make([]string, fontObject)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	var kids strings.Builder
	for index := 0; index < pageCount; index++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+index)
		objects[2+index] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObject, contentObject)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pageCount)
	objects[contentObject-1] = pdfStream([]byte("BT /F1 12 Tf 72 720 Td (page) Tj ET"))
	objects[fontObject-1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	return serializePDF(objects, "")
}

func generateCompressedContentPDF(content []byte) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		flatePDFStream(content),
	}
	return serializePDF(objects, "")
}

func generateCompressedTextOutputBomb(textBytes int) []byte {
	content := make([]byte, 0, textBytes+32)
	content = append(content, "BT /F1 12 Tf ("...)
	content = append(content, bytes.Repeat([]byte{'A'}, textBytes)...)
	content = append(content, ") Tj ET"...)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		flatePDFStream(content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	return serializePDF(objects, "")
}

func generateUniGBFormPDF(text string) []byte {
	pageContent := []byte("/X1 Do")
	formContent := []byte(fmt.Sprintf("BT /F0 11 Tf 0 0 Td <%s> Tj ET", utf16BEHex(text)))
	compressedForm := deflate(formContent)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F0 5 0 R >> /XObject << /X1 7 0 R >> >> /Contents 4 0 R >>",
		flatePDFStream(pageContent),
		"<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [6 0 R] >>",
		qualificationCIDFontObject,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 300 100] /Resources << /Font << /F0 5 0 R >> >> /Filter /FlateDecode /Length %d >>\nstream\n%sendstream", len(compressedForm), compressedForm),
	}
	return serializePDF(objects, "")
}

func generateRepeatedFormOutputBomb(textBytes, invocations int) []byte {
	pageContent := bytes.Repeat([]byte("/X1 Do "), invocations)
	formContent := make([]byte, 0, textBytes+32)
	formContent = append(formContent, "BT /F1 12 Tf ("...)
	formContent = append(formContent, bytes.Repeat([]byte{'A'}, textBytes)...)
	formContent = append(formContent, ") Tj ET"...)
	compressedForm := deflate(formContent)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X1 5 0 R >> >> /Contents 4 0 R >>",
		flatePDFStream(pageContent),
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 300 100] /Resources << /Font << /F1 6 0 R >> >> /Filter /FlateDecode /Length %d >>\nstream\n%sendstream", len(compressedForm), compressedForm),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	return serializePDF(objects, "")
}

func generateObjectXRefStreamPDF(t *testing.T, text string) []byte {
	t.Helper()
	var output bytes.Buffer
	output.WriteString("%PDF-1.7\n%\x80\x80\x80\x80\n")
	offsets := make(map[int]int)

	compressedObjects := []struct {
		number int
		value  string
	}{
		{4, "<< /Type /Catalog /Pages 5 0 R >>"},
		{5, "<< /Type /Pages /Kids [6 0 R] /Count 1 >>"},
		{6, "<< /Type /Page /Parent 5 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F0 7 0 R >> >> /Contents 8 0 R >>"},
	}
	var objectIndex, objectBody strings.Builder
	for _, object := range compressedObjects {
		fmt.Fprintf(&objectIndex, "%d %d ", object.number, objectBody.Len())
		objectBody.WriteString(object.value)
		objectBody.WriteByte(' ')
	}
	objectStream := append([]byte(objectIndex.String()), []byte(objectBody.String())...)
	offsets[1] = output.Len()
	writeIndirectStream(&output, 1,
		fmt.Sprintf("/Type /ObjStm /N %d /First %d /Filter /FlateDecode", len(compressedObjects), objectIndex.Len()),
		deflate(objectStream))

	writeObject := func(number int, value string) {
		offsets[number] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", number, value)
	}
	writeObject(7, "<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /Identity-H /DescendantFonts [10 0 R] /ToUnicode 9 0 R >>")
	content := fmt.Sprintf("BT /F0 11 Tf 72 720 Td <%s> Tj ET", utf16BEHex(text))
	offsets[8] = output.Len()
	writeIndirectStream(&output, 8, "/Filter /FlateDecode", deflate([]byte(content)))
	offsets[9] = output.Len()
	writeIndirectStream(&output, 9, "/Filter /FlateDecode", deflate([]byte(toUnicodeCMap([]string{text}))))
	writeObject(10, qualificationCIDFontObject)

	xrefOffset := output.Len()
	offsets[2] = xrefOffset
	const objectCount = 11
	var xref bytes.Buffer
	for number := 0; number < objectCount; number++ {
		switch number {
		case 0, 3:
			writeXRefEntry(&xref, 0, 0, 65535)
		case 4, 5, 6:
			writeXRefEntry(&xref, 2, 1, uint16(number-4))
		default:
			writeXRefEntry(&xref, 1, uint32(offsets[number]), 0)
		}
	}
	writeIndirectStream(&output, 2,
		fmt.Sprintf("/Type /XRef /Size %d /W [1 4 2] /Index [0 %d] /Root 4 0 R /Filter /FlateDecode", objectCount, objectCount),
		deflate(xref.Bytes()))
	fmt.Fprintf(&output, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return output.Bytes()
}

func writeXRefEntry(output *bytes.Buffer, kind byte, field uint32, tail uint16) {
	_ = output.WriteByte(kind)
	var value [4]byte
	binary.BigEndian.PutUint32(value[:], field)
	_, _ = output.Write(value[:])
	var last [2]byte
	binary.BigEndian.PutUint16(last[:], tail)
	_, _ = output.Write(last[:])
}

func writeIndirectStream(output *bytes.Buffer, number int, dictionary string, data []byte) {
	fmt.Fprintf(output, "%d 0 obj\n<< %s /Length %d >>\nstream\n", number, dictionary, len(data))
	_, _ = output.Write(data)
	output.WriteString("\nendstream\nendobj\n")
}

func flatePDFStream(data []byte) string {
	compressed := deflate(data)
	return fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%sendstream", len(compressed), compressed)
}

func deflate(data []byte) []byte {
	var output bytes.Buffer
	writer := zlib.NewWriter(&output)
	_, _ = writer.Write(data)
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return output.Bytes()
}
