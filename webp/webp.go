// Package webp provides WebP image processing – encode, decode, and manipulate WebP images and sticker metadata.
package webp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
)

// ExifHeader is the TIFF/EXIF header WhatsApp uses to store sticker metadata.
// Structure breakdown:
// - 0x49, 0x49: Little-endian byte order marker ("II")
// - 0x2A, 0x00: TIFF magic number (42)
// - 0x08, 0x00, 0x00, 0x00: Offset to first IFD (8 bytes)
// - 0x01, 0x00: Number of IFD entries (1)
// - 0x41, 0x57: Tag ID (custom "AW" tag for WhatsApp - 0x5741)
// - 0x07, 0x00: Type (7 = UNDEFINED/bytes)
// - 0x00, 0x00, 0x00, 0x00: Count/length (placeholder, updated with actual length)
// - 0x16, 0x00, 0x00, 0x00: Offset to data (22 bytes = 0x16)
var ExifHeader = [22]byte{
	0x49, 0x49, 0x2A, 0x00, // Little-endian TIFF
	0x08, 0x00, 0x00, 0x00, // Offset to IFD
	0x01, 0x00, // Number of entries
	0x41, 0x57, // Tag ID (WhatsApp custom)
	0x07, 0x00, // Type (UNDEFINED)
	0x00, 0x00, 0x00, 0x00, // Count (to be filled)
	0x16, 0x00, 0x00, 0x00, // Offset to data
}

// ExifStickerMetadata represents the metadata serialized inside the EXIF chunk.
type ExifStickerMetadata struct {
	PackID              string   `json:"sticker-pack-id"`
	PackName            string   `json:"sticker-pack-name"`
	Publisher           string   `json:"sticker-pack-publisher"`
	Emojis              []string `json:"emojis,omitempty"`
	AndroidAppStoreLink *string  `json:"android-app-store-link,omitempty"`
	IOSAppStoreLink     *string  `json:"ios-app-store-link,omitempty"`
}

// buildExif builds the EXIF buffer with the header and JSON metadata.
func buildExif(metadata *ExifStickerMetadata) ([]byte, error) {
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	exif := append([]byte(nil), ExifHeader[:]...)
	exif = append(exif, jsonBytes...)

	// Write the JSON length at offset 14 (little-endian u32)
	binary.LittleEndian.PutUint32(exif[14:18], uint32(len(jsonBytes)))

	return exif, nil
}

// isWhatsAppStickerExif checks if EXIF data starts with the WhatsApp sticker header (TIFF LE + "AW" tag).
func isWhatsAppStickerExif(exifBytes []byte) bool {
	if len(exifBytes) < 12 {
		return false
	}
	return bytes.Equal(exifBytes[0:4], []byte{0x49, 0x49, 0x2A, 0x00}) &&
		bytes.Equal(exifBytes[10:12], []byte{0x41, 0x57})
}

// WebPChunk represents a single WebP RIFF chunk.
type WebPChunk struct {
	Type [4]byte
	Data []byte
}

// parseWebP parses the WebP RIFF container into standard chunks.
func parseWebP(data []byte) ([]WebPChunk, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("webp data too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("invalid WebP signature")
	}

	var chunks []WebPChunk
	offset := 12
	for offset < len(data) {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("truncated chunk header")
		}
		var chunkType [4]byte
		copy(chunkType[:], data[offset:offset+4])

		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		offset += 8

		if offset+int(chunkSize) > len(data) {
			return nil, fmt.Errorf("truncated chunk data")
		}

		chunkData := data[offset : offset+int(chunkSize)]
		offset += int(chunkSize)
		if chunkSize%2 != 0 {
			offset++ // padding byte
		}

		chunks = append(chunks, WebPChunk{
			Type: chunkType,
			Data: chunkData,
		})
	}
	return chunks, nil
}

// serializeWebP assembles a WebP RIFF file from a list of chunks.
func serializeWebP(chunks []WebPChunk) []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	buf.Write([]byte{0, 0, 0, 0}) // Placeholder for size
	buf.WriteString("WEBP")

	for _, chunk := range chunks {
		buf.Write(chunk.Type[:])
		var sizeBuf [4]byte
		binary.LittleEndian.PutUint32(sizeBuf[:], uint32(len(chunk.Data)))
		buf.Write(sizeBuf[:])
		buf.Write(chunk.Data)
		if len(chunk.Data)%2 != 0 {
			buf.WriteByte(0)
		}
	}

	res := buf.Bytes()
	// Update RIFF size
	binary.LittleEndian.PutUint32(res[4:8], uint32(len(res)-8))
	return res
}

// getSimpleWebPDimensions extracts canvas width, height, and alpha usage from VP8 (lossy) or VP8L (lossless) chunks.
func getSimpleWebPDimensions(chunks []WebPChunk) (width uint32, height uint32, hasAlpha bool, err error) {
	for _, chunk := range chunks {
		switch chunk.Type {
		case [4]byte{'V', 'P', '8', ' '}:
			if len(chunk.Data) < 10 {
				return 0, 0, false, fmt.Errorf("VP8 chunk too short")
			}
			// Check keyframe start code
			if chunk.Data[3] != 0x9d || chunk.Data[4] != 0x01 || chunk.Data[5] != 0x2a {
				return 0, 0, false, fmt.Errorf("invalid VP8 keyframe start code")
			}
			w := uint32(chunk.Data[6]) | (uint32(chunk.Data[7]) << 8)
			h := uint32(chunk.Data[8]) | (uint32(chunk.Data[9]) << 8)
			return w & 0x3fff, h & 0x3fff, false, nil
		case [4]byte{'V', 'P', '8', 'L'}:
			if len(chunk.Data) < 5 {
				return 0, 0, false, fmt.Errorf("VP8L chunk too short")
			}
			if chunk.Data[0] != 0x2f {
				return 0, 0, false, fmt.Errorf("invalid VP8L signature")
			}
			val := binary.LittleEndian.Uint32(chunk.Data[1:5])
			w := (val & 0x3fff) + 1
			h := ((val >> 14) & 0x3fff) + 1
			alpha := ((val >> 28) & 1) != 0
			return w, h, alpha, nil
		}
	}
	return 0, 0, false, fmt.Errorf("no VP8 or VP8L chunk found")
}

// createVP8XChunk constructs a new VP8X extended WebP chunk.
func createVP8XChunk(width, height uint32, hasAlpha, hasAnimation, hasEXIF bool) WebPChunk {
	var data [10]byte
	var flags byte
	if hasAlpha {
		flags |= 1 << 2 // Bit 2: Alpha (A)
	}
	if hasEXIF {
		flags |= 1 << 3 // Bit 3: EXIF metadata (E)
	}
	if hasAnimation {
		flags |= 1 << 5 // Bit 5: Animation (M)
	}

	data[0] = flags
	// canvas width - 1 is stored in 24 bits (bytes 4-6)
	wMinus1 := width - 1
	data[4] = byte(wMinus1)
	data[5] = byte(wMinus1 >> 8)
	data[6] = byte(wMinus1 >> 16)

	// canvas height - 1 is stored in 24 bits (bytes 7-9)
	hMinus1 := height - 1
	data[7] = byte(hMinus1)
	data[8] = byte(hMinus1 >> 8)
	data[9] = byte(hMinus1 >> 16)

	return WebPChunk{
		Type: [4]byte{'V', 'P', '8', 'X'},
		Data: data[:],
	}
}

// injectEXIF injects or replaces the EXIF chunk in the parsed WebP chunks.
func injectEXIF(webpData []byte, exifData []byte) ([]byte, error) {
	chunks, err := parseWebP(webpData)
	if err != nil {
		return nil, err
	}

	var vp8xIdx = -1
	var exifIdx = -1

	for i, c := range chunks {
		switch c.Type {
		case [4]byte{'V', 'P', '8', 'X'}:
			vp8xIdx = i
		case [4]byte{'E', 'X', 'I', 'F'}:
			exifIdx = i
		}
	}

	newExifChunk := WebPChunk{
		Type: [4]byte{'E', 'X', 'I', 'F'},
		Data: exifData,
	}

	var newChunks []WebPChunk
	if vp8xIdx != -1 {
		// VP8X is present, update its EXIF flag
		if len(chunks[vp8xIdx].Data) < 10 {
			return nil, fmt.Errorf("invalid VP8X chunk size")
		}
		// Set EXIF flag (bit 3)
		chunks[vp8xIdx].Data[0] |= (1 << 3)

		// Copy chunks, excluding the old EXIF chunk
		for i, c := range chunks {
			if i != exifIdx {
				newChunks = append(newChunks, c)
			}
		}

		// Find the index of the first XMP chunk to insert EXIF right before it
		var newXmpIdx = -1
		for i, c := range newChunks {
			if c.Type == [4]byte{'X', 'M', 'P', ' '} {
				newXmpIdx = i
				break
			}
		}

		if newXmpIdx != -1 {
			// Insert before XMP safely
			temp := make([]WebPChunk, 0, len(newChunks)+1)
			temp = append(temp, newChunks[:newXmpIdx]...)
			temp = append(temp, newExifChunk)
			temp = append(temp, newChunks[newXmpIdx:]...)
			newChunks = temp
		} else {
			// Append to the end
			newChunks = append(newChunks, newExifChunk)
		}
	} else {
		// VP8X is not present, we need to create it
		width, height, hasAlpha, err := getSimpleWebPDimensions(chunks)
		if err != nil {
			return nil, fmt.Errorf("failed to get simple webp dimensions: %w", err)
		}
		vp8x := createVP8XChunk(width, height, hasAlpha, false, true)

		// Construct new chunk list: VP8X first, then original chunks, then EXIF
		newChunks = append(newChunks, vp8x)
		newChunks = append(newChunks, chunks...)
		newChunks = append(newChunks, newExifChunk)
	}

	return serializeWebP(newChunks), nil
}

// extractEXIF extracts the EXIF chunk payload from a WebP image.
func extractEXIF(webpData []byte) ([]byte, error) {
	chunks, err := parseWebP(webpData)
	if err != nil {
		return nil, err
	}
	for _, c := range chunks {
		if c.Type == [4]byte{'E', 'X', 'I', 'F'} {
			return c.Data, nil
		}
	}
	return nil, nil
}

// AddStickerMetadataWithOptions adds sticker metadata to a WebP image with advanced options.
func AddStickerMetadataWithOptions(webpData []byte, packName, publisher string, emojis []string, androidLink, iosLink *string) ([]byte, error) {
	packID := uuid.New().String()
	meta := &ExifStickerMetadata{
		PackID:              packID,
		PackName:            packName,
		Publisher:           publisher,
		Emojis:              emojis,
		AndroidAppStoreLink: androidLink,
		IOSAppStoreLink:     iosLink,
	}

	exifData, err := buildExif(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to build exif: %w", err)
	}

	return injectEXIF(webpData, exifData)
}

// AddStickerMetadata adds standard sticker metadata (pack name and publisher) to a WebP image.
func AddStickerMetadata(webpData []byte, packName, publisher string) ([]byte, error) {
	return AddStickerMetadataWithOptions(webpData, packName, publisher, nil, nil, nil)
}

// GetStickerMetadata extracts and decodes sticker metadata from a WebP image if present.
func GetStickerMetadata(webpData []byte) (*ExifStickerMetadata, error) {
	exifBytes, err := extractEXIF(webpData)
	if err != nil {
		return nil, err
	}
	if exifBytes == nil {
		return nil, nil
	}

	if !isWhatsAppStickerExif(exifBytes) {
		return nil, nil
	}

	if len(exifBytes) <= len(ExifHeader) {
		return nil, nil
	}

	jsonBytes := exifBytes[len(ExifHeader):]
	var meta ExifStickerMetadata
	if err := json.Unmarshal(jsonBytes, &meta); err != nil {
		return nil, nil // Return nil, nil if malformed or different format
	}

	return &meta, nil
}

// WriteStickerMetadata modifies the sticker file with metadata and writes to a new file.
func WriteStickerMetadata(inputPath, packName, author string) (string, error) {
	webpData, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read webp: %w", err)
	}

	outputData, err := AddStickerMetadata(webpData, packName, author)
	if err != nil {
		return "", fmt.Errorf("failed to add sticker metadata: %w", err)
	}

	outputPath := inputPath + ".metadata.webp"
	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return "", fmt.Errorf("failed to write output webp: %w", err)
	}

	return outputPath, nil
}
