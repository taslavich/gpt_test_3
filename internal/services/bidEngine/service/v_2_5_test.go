package bidEngine

import (
	"log"
	"net/url"
	"testing"
)

func TestMain(t *testing.T) {

	addr := "https://u-48702.daleelerah.info/api/rtb-pops/go?id=30691032339541934&sig=40c262c3a53f8a0ca00a90f3202e6b&u=aHR0cHM6Ly92aWRlb3Nob3Uub25saW5lL1NHVDFiNTVUP2Nvc3Q9Ni45JmN1cnJlbmN5PXVzZCZleHRlcm5hbF9pZD17Y2xpY2tfaWR9JmNyZWF0aXZlX2lkPXtjcmVhdGl2ZV9pZH0mYWRfY2FtcGFpZ25faWQ9e2NhbXBhaWduX2lkfSZzb3VyY2U9e3NvdXJjZV9pZH0mbW9kZWxfcHJpY2U9e21vZGVsX3ByaWNlfSZwcmljZT02Ljkmc3ViX2lkPXtzdWJfaWR9JmNvdW50cnk9e2NvdW50cnl9Jm9zPXtvc30mYnJvd3Nlcj17YnJvd3Nlcn0mcGxhdGZvcm09e3BsYXRmb3JtfSZxdWFsaXR5PXtxdWFsaXR5fSZjYXRlZ29yeT17Y2F0ZWdvcnl9JnN1YnNfYWdlPXtzdWJzX2FnZX0mY2F0ZWdvcnlfc3JjPXtjYXRlZ29yeV9zcmN9JmNpdHk9e2NpdHl9JnJlZ2lvbj17cmVnaW9ufSZjcmVvPXtjcmVvfSZlcmlkPUxhdGdCY0tUbg%3D%3D"
	encode := url.QueryEscape(addr)

	log.Println(encode)
	gotAdrr, err := url.QueryUnescape(encode)
	if err != nil {
		log.Fatalf("PIZDEC 2")
	}

	if addr == gotAdrr {
		log.Println("OK")
	}
}
