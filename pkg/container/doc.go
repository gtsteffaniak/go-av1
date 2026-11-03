// Package container parses AV1 bitstreams from common containers (ISOBMFF/AVIF,
// MP4/MOV, Matroska/WebM) and builds low-overhead temporal units suitable for
// the root goav1 decoder.
//
// The isobmff subpackage (MIT, adapted from KarpelesLab/goavif) implements
// HEIF/AVIF item layout. See NOTICE in this directory.
package container
