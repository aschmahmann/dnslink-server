package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

func main()  {
	s := dns.NewServeMux()
	s.Handle("git", &GitHandler{})
}

type GitHandler struct {}

func (g GitHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	n := r.Question[0].Name
	t := r.Question[0].Qtype

	if t != dns.TypeTXT {
		_ = w.Close()
		return
	}

	parts := strings.Split(n, ".")
	if len(parts) == 1 {
		_ = w.Close()
		return
	}

	head := parts[0]
	parts = parts[1:]

	parts[len(parts) - 1] = "http:/"
	_ = sort.Reverse(sort.StringSlice(parts))
	repo := strings.Join(parts,"/")

	res, err  := exec.Command("git", "ls-remote", repo).Output()
	if err != nil {
		_ = w.Close()
		return
	}

	scan := bufio.NewScanner(bytes.NewReader(res))
	for scan.Scan() {
		txt := scan.Text()
		if strings.Contains(txt, fmt.Sprintf("refs/heads/%s", head)) {
			gitHash := txt[0:strings.Index(txt," ")]

			_ = w.WriteMsg(&dns.Msg{Answer: []dns.RR{&dns.TXT{
				Hdr: dns.RR_Header{
					Ttl:      5, // seconds
				},
				Txt: []string{gitHash},
			}}})
			return
		}
	}

	if err := scan.Err(); err != nil {
		// Handle the error
		_ = w.Close()
		return
	}
}

var _ dns.Handler = (*GitHandler)(nil)
