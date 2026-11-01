// Command icons writes the Windows icon resource objects that the Go linker
// embeds in StremioSmartSearch.exe, so Explorer, the taskbar and shortcuts show
// the app logo.
//
// Run it from the repository root after changing assets/icon.ico:
//
//	go run ./tools/icons
//
// It writes cmd/smart-search/rsrc_windows_{amd64,arm64}.syso. Go links any
// .syso file in a package directory whose name matches the target platform, so
// a normal `go build` picks them up with no extra tools.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langEnUS    = 0x0409
)

// COFF machine types and the relocation each uses for a 32-bit image-relative
// address (ADDR32NB), which resource data entries need.
var machines = []struct {
	arch       string
	machine    uint16
	relocation uint16
}{
	{"amd64", 0x8664, 0x0003},
	{"arm64", 0xAA64, 0x0002},
}

type iconImage struct {
	width, height, colors uint8
	planes, bitCount      uint16
	data                  []byte
}

func main() {
	in := flag.String("ico", "assets/icon.ico", "source .ico file")
	out := flag.String("out", "cmd/smart-search", "directory for the .syso files")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "icons:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	images, err := parseICO(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	for _, m := range machines {
		obj := buildObject(images, m.machine, m.relocation)
		path := filepath.Join(out, "rsrc_windows_"+m.arch+".syso")
		if err := os.WriteFile(path, obj, 0644); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%d bytes, %d images)\n", path, len(obj), len(images))
	}
	return nil
}

func parseICO(b []byte) ([]iconImage, error) {
	if len(b) < 6 || binary.LittleEndian.Uint16(b[0:]) != 0 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		return nil, errors.New("not an icon file")
	}
	count := int(binary.LittleEndian.Uint16(b[4:]))
	if count == 0 || len(b) < 6+16*count {
		return nil, errors.New("icon directory is empty or truncated")
	}
	images := make([]iconImage, count)
	for i := range images {
		e := b[6+16*i:]
		size := binary.LittleEndian.Uint32(e[8:])
		offset := binary.LittleEndian.Uint32(e[12:])
		if uint64(offset)+uint64(size) > uint64(len(b)) {
			return nil, fmt.Errorf("image %d runs past the end of the file", i+1)
		}
		images[i] = iconImage{
			width: e[0], height: e[1], colors: e[2],
			planes:   binary.LittleEndian.Uint16(e[4:]),
			bitCount: binary.LittleEndian.Uint16(e[6:]),
			data:     b[offset : offset+size],
		}
	}
	return images, nil
}

// buildObject lays out a COFF object with one .rsrc section holding the
// resource tree type -> id -> language -> data for RT_ICON (one per image)
// and RT_GROUP_ICON (id 1, the icon Windows uses for the executable).
func buildObject(images []iconImage, machine, relocType uint16) []byte {
	group := groupIcon(images)
	type leaf struct {
		id   uint16
		data []byte
	}
	types := []struct {
		id     uint16
		leaves []leaf
	}{{id: rtIcon}, {id: rtGroupIcon, leaves: []leaf{{1, group}}}}
	for i, img := range images {
		types[0].leaves = append(types[0].leaves, leaf{uint16(i + 1), img.data})
	}

	dirSize := func(entries int) int { return 16 + 8*entries }
	// Directory tables first, then data entries, then the data itself.
	offset := dirSize(len(types))
	typeDir := make([]int, len(types))
	for i, t := range types {
		typeDir[i] = offset
		offset += dirSize(len(t.leaves))
	}
	nameDir := make([][]int, len(types))
	for i, t := range types {
		for range t.leaves {
			nameDir[i] = append(nameDir[i], offset)
			offset += dirSize(1)
		}
	}
	dataEntry := make([][]int, len(types))
	for i, t := range types {
		for range t.leaves {
			dataEntry[i] = append(dataEntry[i], offset)
			offset += 16
		}
	}
	dataAt := make([][]int, len(types))
	for i, t := range types {
		for _, l := range t.leaves {
			offset = align(offset, 8)
			dataAt[i] = append(dataAt[i], offset)
			offset += len(l.data)
		}
	}
	section := make([]byte, align(offset, 8))
	le := binary.LittleEndian
	writeDir := func(at, entries int) {
		le.PutUint16(section[at+14:], uint16(entries)) // NumberOfIdEntries
	}
	writeDir(0, len(types))
	var relocs []int
	for i, t := range types {
		e := dirSize(0) + 8*i
		le.PutUint32(section[e:], uint32(t.id))
		le.PutUint32(section[e+4:], uint32(typeDir[i])|0x80000000)
		writeDir(typeDir[i], len(t.leaves))
		for j, l := range t.leaves {
			e := typeDir[i] + dirSize(0) + 8*j
			le.PutUint32(section[e:], uint32(l.id))
			le.PutUint32(section[e+4:], uint32(nameDir[i][j])|0x80000000)
			writeDir(nameDir[i][j], 1)
			e = nameDir[i][j] + dirSize(0)
			le.PutUint32(section[e:], langEnUS)
			le.PutUint32(section[e+4:], uint32(dataEntry[i][j]))
			d := dataEntry[i][j]
			le.PutUint32(section[d:], uint32(dataAt[i][j])) // relocated to an RVA
			le.PutUint32(section[d+4:], uint32(len(l.data)))
			relocs = append(relocs, d)
			copy(section[dataAt[i][j]:], l.data)
		}
	}

	const fileHeader, sectionHeader, relocSize, symbolSize = 20, 40, 10, 18
	rawAt := fileHeader + sectionHeader
	relocAt := rawAt + len(section)
	symAt := relocAt + relocSize*len(relocs)

	var out bytes.Buffer
	w := func(v any) { _ = binary.Write(&out, le, v) }
	w(machine)
	w(uint16(1)) // sections
	w(uint32(0)) // timestamp, zero for reproducible builds
	w(uint32(symAt))
	w(uint32(1)) // symbols
	w(uint16(0)) // optional header size
	w(uint16(0)) // characteristics
	out.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0)) // virtual size
	w(uint32(0)) // virtual address
	w(uint32(len(section)))
	w(uint32(rawAt))
	w(uint32(relocAt))
	w(uint32(0)) // line numbers
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // initialized data, readable
	out.Write(section)
	for _, r := range relocs {
		w(uint32(r))
		w(uint32(0)) // symbol 0: the .rsrc section
		w(relocType)
	}
	out.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0)) // value
	w(uint16(1)) // section number
	w(uint16(0)) // type
	w(uint8(3))  // storage class: static
	w(uint8(0))  // aux symbols
	w(uint32(4)) // empty string table
	return out.Bytes()
}

func groupIcon(images []iconImage) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w(uint16(0))
	w(uint16(1))
	w(uint16(len(images)))
	for i, img := range images {
		w(img.width)
		w(img.height)
		w(img.colors)
		w(uint8(0))
		w(img.planes)
		w(img.bitCount)
		w(uint32(len(img.data)))
		w(uint16(i + 1))
	}
	return b.Bytes()
}

func align(n, to int) int { return (n + to - 1) / to * to }
