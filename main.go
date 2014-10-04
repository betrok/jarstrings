package main

import (
	"os"
	"io"
	"log"
	"flag"
	"bytes"
	"regexp"
	"strings"
	"archive/zip"
	"encoding/binary"
)

var (
	type_len = map[byte]int {
		//1: 2+var -- string
		3:	4,	//int32
		4:	4,	//float
		5:	8,	//int64
		6:	8,	//double
		7:	2,	//class reference
		8:	2,	//string reference
		9:	4,	//field reference
		10:	4,	//method reference
		11:	4,	//interface reference
		12:	4,	//name or type descriptor
	}
	
	find *regexp.Regexp
	replace_flag bool
	input, replace, output string
	
	skip_meta bool
)

type preConstHeader struct {
	Magic uint32
	MinVers uint16
	MajVers uint16
	ConstCount uint16
}

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
	
	flag.BoolVar(&skip_meta, "s", false, "")
	flag.Usage = func() { log.Printf(`Usage: %s [options] <file.jar> [find_regexp] [replace_string output.jar]
		
		-s	do not copy META-INF`, os.Args[0]) }
	flag.Parse()
	
	args := flag.Args()
	
	var err error
	switch len(args) {
		case 4:
			replace_flag = true
			output = args[3]
			replace = args[2]
			fallthrough
			
		case 2:
			find, err = regexp.Compile(args[1])
			if(err != nil) { log.Fatal(err) }
			fallthrough
			
		case 1:
			input = args[0]
			
		default:
			flag.Usage()
			os.Exit(1)
	}
	r, err := zip.OpenReader(input)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	var w *zip.Writer
	if(replace_flag) {
		fd, err := os.Create(output)
		if err != nil {
			log.Fatal(err)
		}
		defer fd.Close()
		w = zip.NewWriter(fd)
		defer w.Close()
	}
	
	var header preConstHeader
	var wf io.Writer
	for _, f := range r.File {
		if(strings.HasSuffix(f.Name, "/") || (skip_meta && strings.HasPrefix(f.Name, "META-INF"))) { continue }
		
		if(!strings.HasSuffix(f.Name, ".class")) {
			if(replace_flag) {
				rf, err := f.Open()
				if(err != nil) { log.Fatal(err) }
				wf, err = w.Create(f.Name)
				if(err != nil) { log.Fatal(err) }
				_, err = io.Copy(wf, rf)
				if(err != nil) { log.Fatal(err) }
				rf.Close()
			}
			continue
		}
		
		rf, err := f.Open()
		if(err != nil) { log.Fatal(err) }
		if(replace_flag) {
			wf, err = w.Create(f.Name)
			if(err != nil) { log.Fatal(err) }
		}
		
		err = binary.Read(rf, binary.BigEndian, &header)
		if(err != nil) { log.Fatal(err) }
		
		if(header.Magic != 0xCAFEBABE) { log.Fatal("Isn't 0xCAFEBABE") }
		if(replace_flag) {
			err = binary.Write(wf, binary.BigEndian, &header)
			if(err != nil) { log.Fatal(err) }
		}
		
		first := false
		buf := make([]byte, 8)
		var tag uint8
		var strlen, i uint16
		for i = 1; i < header.ConstCount; i++ {
			err = binary.Read(rf, binary.BigEndian, &tag)
			if(err != nil) { log.Fatal(err) }
			if(replace_flag) {
				err = binary.Write(wf, binary.BigEndian, &tag)
				if(err != nil) { log.Fatal(err) }
			}
			
			if(tag == 1) {
				err = binary.Read(rf, binary.BigEndian, &strlen)
				if(err != nil) { log.Fatal(err) }
				
				if(cap(buf) < int(strlen)) { buf = make([]byte, 0, strlen) }
				_, err := io.ReadFull(rf, buf[:strlen])
				if(err != nil) { log.Fatal(err) }
				
				switch {
					case replace_flag:
						modifed := find.ReplaceAllLiteral(buf[:strlen], []byte(replace))
						if(!bytes.Equal(modifed, buf[:strlen])) {
							if(!first) {
								first = true
								log.Println(f.Name)
							}
							log.Printf("  \"%s\" => \"%s\"", string(buf[:strlen]), string(modifed))
						}
						strlen = uint16(len(modifed))
						err = binary.Write(wf, binary.BigEndian, &strlen)
						if(err != nil) { log.Fatal(err) }
						_, err = wf.Write(modifed)
						if(err != nil) { log.Fatal(err) }
						
					case find != nil:
						if(find.Match(buf[:strlen])) {
							if(!first) {
								first = true
								log.Println(f.Name)
							}
							log.Println("  ", string(buf[:strlen]))
						}
						
					default:
						if(!first) {
							first = true
							log.Println(f.Name)
						}
						log.Println("  ", string(buf[:strlen]))
				}
				
			} else if l, ok := type_len[tag]; ok {
				_, err = io.ReadFull(rf, buf[:l])
				if(err != nil) { log.Fatal(err) }
				if(replace_flag) {
					_, err = wf.Write(buf[:l])
					if(err != nil) { log.Fatal(err) }
				}
				if(tag == 5 || tag == 6) { i++ }
			} else {
				log.Fatal("Unknown tag ", tag)
			}
			if(err != nil) { log.Fatal(err) }
		}
		if(replace_flag) {
			_, err = io.Copy(wf, rf)
			if(err != nil) { log.Fatal(err) }
		}
		
		rf.Close()
		if(first) { log.Println() }
	}
}
