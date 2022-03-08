package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"

	"github.com/miekg/dns"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

func main() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			return
		}
		reqm := &dns.Msg{}
		if err := reqm.Unpack(body); err != nil {
			return
		}

		writer.Header().Set("Content-Type", dohMimeType)
		if err := writeGit(writer, reqm); err != nil {
			return
		}
	})

	err := http.ListenAndServeTLS(":3333", "server.crt", "server.key", nil)
	if err != nil {
		panic(err)
	}
}

const (
	dohMimeType = "application/dns-message"
)

func writeGit(w io.Writer, r *dns.Msg) error {
	n := r.Question[0].Name
	t := r.Question[0].Qtype

	if t != dns.TypeTXT {
		return errors.New("only TXT records are supported")
	}

	head, repo, err := dnsNameToGit(n)
	if err != nil {
		return err
	}

	res, err := exec.Command("git", "ls-remote", repo).Output()
	if err != nil {
		return err
	}

	scan := bufio.NewScanner(bytes.NewReader(res))
	for scan.Scan() {
		txt := scan.Text()
		if strings.Contains(txt, fmt.Sprintf("refs/heads/%s", head)) {
			gitHash := txt[0:strings.Index(txt, "\t")]
			b, err := hex.DecodeString(gitHash)
			if err != nil {
				return err
			}
			mh, err := multihash.Encode(b, multihash.SHA1)
			if err != nil {
				return err
			}
			c := cid.NewCidV1(cid.GitRaw, mh)

			msg := new(dns.Msg)
			msg.SetReply(r)

			t := &dns.TXT{
				Hdr: dns.RR_Header{Name: "localhost.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 5},
				Txt: []string{"dnslink=/ipfs/" + c.String()},
			}

			msg.Answer = append(msg.Answer, t)

			msgb, err := msg.Pack()
			if err != nil {
				return err
			}

			_, err = w.Write(msgb)
			if err != nil {
				return err
			}
			return nil
		}
	}

	if err := scan.Err(); err != nil {
		// Handle the error
		return err
	}
	return nil
}

// dnsNameToGit takes a DNS name and tries to interpret it as a git branch name and repo
// Returns branchName, repo, error
//
// The format is `head.repolocation.git.` listed in the canonical most general to most specific ordering.
// For example: github.com/ipfs/go-ipfs@master -> `master.go-ipfs.ipfs.-.github.com.git.`
//
// Not yet supported: characters valid in path but not within a DNS label (a common one being `.`)
// Only supports HTTPS accessible Git repos
//
// For example, `master.go-ipfs.ipfs.-.github.com.git.` would become return a branch name "master"
// and a repo "https://github.com/ipfs/go-ipfs". This could be resolved as `/ipns/master.go-ipfs.ipfs.-.github.com.git`
//
// TODO: This encoding is not valid DNS since `-` is not a valid DNS label
func dnsNameToGit(name string) (string, string, error) {
	parts := strings.Split(name, ".")
	if len(parts) < 4 {
		return "", "", errors.New("needs more at least 4 domain parts")
	}

	head := parts[0]
	parts = parts[1 : len(parts)-2]
	repo := "https://"
	var repobase, repopath string
	for i, p := range parts {
		if p == "-" {
			repobase = strings.Join(parts[i+1:], ".")
			reverse(parts[:i])
			repopath = strings.Join(parts[:i], "/")
			repo += repobase + "/" + repopath
			break
		}
	}
	if repobase == "" {
		repo = strings.Join(parts, ".")
	}

	return head, repo, nil
}

func reverse(ss []string) {
	last := len(ss) - 1
	for i := 0; i < len(ss)/2; i++ {
		ss[i], ss[last-i] = ss[last-i], ss[i]
	}
}
