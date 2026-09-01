package whatsrook

import (
	"whatsrook/webp"
)

type (
	// ExifStickerMetadata represents the metadata serialized inside the EXIF chunk.
	ExifStickerMetadata = webp.ExifStickerMetadata

	// WebPChunk represents a single WebP RIFF chunk.
	WebPChunk = webp.WebPChunk
)

var (
	// ExifHeader is the TIFF/EXIF header WhatsApp uses to store sticker metadata.
	ExifHeader = webp.ExifHeader

	// AddStickerMetadataWithOptions adds sticker metadata to a WebP image with advanced options.
	AddStickerMetadataWithOptions = webp.AddStickerMetadataWithOptions

	// AddStickerMetadata adds standard sticker metadata to a WebP image.
	AddStickerMetadata = webp.AddStickerMetadata

	// GetStickerMetadata extracts and decodes sticker metadata from a WebP image if present.
	GetStickerMetadata = webp.GetStickerMetadata

	// WriteStickerMetadata modifies the sticker file with metadata and writes to a new file.
	WriteStickerMetadata = webp.WriteStickerMetadata
)
