package tools

import (
	attachmentpkg "github.com/ZihengXiong/GenMult/internal/attachment"
	"github.com/ZihengXiong/GenMult/internal/channel"
)

// toolAttachmentFromBundle converts a normalized bundle to a tools.Attachment.
// Callers must guarantee bundle is already normalized (produced by BundleFromXxx or Normalize()).
func toolAttachmentFromBundle(bundle attachmentpkg.Bundle) Attachment {
	return Attachment{
		Type:        bundle.Type,
		Base64:      bundle.Base64,
		Path:        bundle.Path,
		URL:         bundle.URL,
		PlatformKey: bundle.PlatformKey,
		ContentHash: bundle.ContentHash,
		Name:        bundle.Name,
		Mime:        bundle.Mime,
		Size:        bundle.Size,
		Metadata:    bundle.Metadata,
	}
}

func toolAttachmentFromChannelAttachment(att channel.Attachment) Attachment {
	return toolAttachmentFromBundle(channel.BundleFromAttachment(att))
}
