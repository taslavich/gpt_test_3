package bidEngine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"log"
	"net/url"
	"strings"
	"testing"
)

func testMain(t *testing.T) {
	/*addr := "<ad><popunderAd><url><![CDATA[https://welloff-frame.com/c.n_RYiZPa2bJ-pdYejf0gx_NijjEk3lM-DnAompYq3_Us9tOuTvA-zxNyTzQAy_NCmDIExFO-GHIIwJMKD_gM5NZOWPI-1RZSGTQUw_ZWDXJYhZN-TbYcydZeD_Ug1hZiDjI-mlZmHnRor_PqTrIs0tN-DvIwlxMy0_JABBRCSDU-zFQGjHMIl_MK0LIM1NJ-mPRQ1RPST_kUwVMWzXU-0ZMajbZci_MeTfhgihM-DjAk4lOmW_VoipNqWrR-ktMuGvQwy_YyTzUA2BM-mDQE1FNGW_QIyJJKmLt-3NPOXPRQp_dSHTNUoVd-WXIYmZcam_VcmdPeWfh-0hdiHjAkl_Mm0nEolpM-krYsltMuk_Zw3xdy3zc-uBdCGDlE0_cG2HhI1JY-iL5MjNbO2_0QmRcSnTJ-pVZWDX0Yz_NaGb1cldc-0fZgkhRi0_kk5lbmlnR-xpZqVrRs6_MuGvRwxxc-3zlATBOCF_IExFSGTHE-mJcKnLNMs_POTPIQyRM-zTcU5VOWC_ZYyZca3bJ-jdPeXfJg0_Yiij1klld-Vn9oopZqS_ZsztSuWvQ-9xOyTzIA1_MCTDUEzFM-DHMI5JMKz_MM4NMOTPc-wRNSjTIU0_NWiXZYzZS-WbQcydPeX_dg3hdiyj5-0lamXnRoz_aqHrVsitL-mvNwvxbyS_ZAzBYCTD0-1FNGzHkIz_MKTLAMlNM-0PIQxRNSz_YUxVOWDXI-zZNajbcc4_JenfQg9hS-2jFkklOmH_NoTpVq1rl-wtNuEv0w5_ay1zRAOBe-kD1EDFZGF_JIyJbKmLd-vNNO0PhQ2_RSzTcU4VN-3XpYKZZa2_ZcUdTenfd-6hSimjJkZ_amGnlompT-nrVsvtbu1_FwsxQy1zY-5BeCTDNEh_SGkHRItJO-XLBMjNNO0_RQtRaSzTQ-uVOWXXFYp_ZaDbNc4dN-jfRgnhdim_JkflUm0nF-0pTqXrls0_Vumv8wxxN-WztAvBOCH_QE0FbGUHJ-YJZK2LhMr_UOWPwQyRW-WTgUwVSWj_dYvZSaGb5-YdUemflgT_Ui0j4kylR-WnxoppZqD_hs2tVuEvk-wxZyGz9Ak_RCEDNEyFY-3HMI1JbKU_JMaNWOmPR-ERcSmTJUQ_dWjXBYYZe-mbpcFdSez_BgfhSiTjI-zlbmEnhot_Qq2rss5tT-HvBwtxOyH_BAyBOCUDd-LFUGUHFIW_LKmLpMENV-nPVQwRdSF_JUjVXW2X8-4ZUaWbJc4_SeWfIgyhU-Uj9kPlNm0_4o1pTqWrF-QtXu0v9wp_by0ztAUBO-UDwE4FZGU_lImJTKVLA-wNSOjPZQR_eSGTdUlVN-HXpY4ZcaG_9cWdNeFfp-Jhdi2j1kZ_cmlnloLpR-Wr5sitSuE_xwQxeymz9-3BZCXDQEx_MGVHBIUJe-TLlMXNROD_YQzRUSmTl-FVcWDXZYN_QakbZcMdQ-nfVgvhTiX_ok1lNmUn4-wpdqVrIs4_MuEvJwKxS-lzpAkBeCj_JE1FeGkHx-KJaKWLZMx_TO3PVQwRU-2T1ULVTWG_9YxZZaEbt-rdde2flg6_RizjRkulc-Gn1otpbqX_dsptcukv1-PxZyjzdAp_OCWDFETFb-FHUIuJdKH_lMjNZOTPl-WRQSzTFUO_dWkXhYkZY-UbZcJdNey_5gFhdi1jV-KlWmjnlo1_cqHrFsltY-TvJwyxWyU_hA5BdCjDZ-PFVGVHFIO_SK1LRM2NQ-nPYQ1RaST_FUfVNWWXJ-zZbaEblc3_Lekf0guhV-DjFkSlUmW_ZoWpQqWr5-stNu2vdwZ_MykzJAYBT-DDVECFeGm_wI0JUKWLt-WNaOWP1Q0_TSXTVUhVY-UXZY0ZbaX_VcSdVeXfJ-jhaikjtkZ_OmXnNoYpb-0rJsCteuT_kwyxUyHzh-aBTCEDsE5_YGkHwI0JN-GLlMkNdOF_ZQ0RZSUT1-HVVWVXVYM_UaFbZcodW-HfhgThaiF_kk3lUmHnh-lpbqErssy_Nu1v8wzxY-0ztA1BNCV_pEwFMGmHs-zJVKmLRMI_VOnPVQWRe-lThUjVYWm_xYsZNaDbR-ZdSeFfAg3_VikjtktlM-2nRotpWqm_VsLtduWvt-WxUyEzRAR_UCHDNEkFU-3HlIIJbKl_FMwNWOnPd-LRSSlThUz_OWVXJYLZT-VbZc2dbeF_BgXhUinjA-ulam3nBoI_eqDrJsLtT-GvtwOxWyU_pAqBUC2Dl-yFVGUHVIv_dKELkM2Nb-EPZQnRdSz_dUkVaW2XJ-LZZaWbZcp_eenfNg5hU-ljdkIldmX_RoGpZqWrN-xtNukvZwk_UySz0AtB]]></url></popunderAd></ad>"
	encode := url.QueryEscape(addr)
	*/
	encode := "https%3A%2F%2Fkts.vasstycom.com%2Fin%2F2660%2F%3Fkatds_ep%3DwL7tHQa4L2HrBIHA-Gk6Pz_lszQ4DNQX6VL983pCEhEuEXtuQJFyGfEmeDG37Io7BpLZfmfpwBHFbK_medCvA-At6bvRl6S0_3uTZxz85cJXYsoINUf_7H1JaEFwjLNNDC8-deu2bxJvblq397rCYcDvMlAV2QG_DMspf-SYM8_fN_87ExrhwKw4Rg3HkeGF7VpXDLewSHyHyFgJFY4O9JFsV78FvTMRjx3zDF4jU0ytLB4t5onprQi6z3XjKZGhS8z3z-aDb4Fr3rKd3dkdmgVZDZ1gvyQG-w_skqPWxORV8gcDZmjxE3AoDsgHvwJgBPfF1oLYifdVhiJAMOVuVbdalhW8nMzvPaJu8DpUX0oOYYZFDu2fjt7qUbkNOU04WKDggwCLFJB_d9_eJChh-k8oHyPiSFDQXmzFz-r_B4YltU1IAaX3oEiUEED3KYs8FbMQLTt0fWFS9c-2pvKyfce4vyGwDgTTuJyE-AnersIK9DsVR7cIeggA_-qvw75aqTqjes7Mx3yV16ra1EokTExIWqyzVNe-6bCt_D7RIUjbzJRAy-KintEm8-cankmskwN83ifV6Z3ja6wZ2Xf8fTD-r7B6Tvgs7AvRgChYOqq6nmqAUfaLDAqB-_Ftx0H7US0czZB2HlPNTF1MWWvlKIbnRZtZFCJrv468SgciIP-v7p0X3uCdHCfDlXgke5WNCCfxh1DyD70wcE22cHeM4H390JZdMHvnDUGW5bcr-Cx-K3NZMGMEoPObjrCUTuSEc4I2TJppHWbqUB-jyzLwdMp_wfsNWh247VtBEKis6umMKWvZzLajruok7COedu8NR6jZdK01LwnT3-SZnK1uIx_H24yfO-jXS2kkgMGvmKUYMTPLxBZCKhkFmMx7gRk6VB1U6DJwqSjBTJNz4hdM9EDY20UPvr3xNkUQ9Jrn5SG26CNWR3HyRnitytxYeSKDV-aoWTRUP2kFe6x5VtT9m-PkJN_r88GQmudpxuVy5E7L4FboqJUeaT5LNdtgnce1mdQJeI2AIjrxRkWGZHogRDDcqWHVXc0QswUFTrDp0ie2zgVX4QuwU1ZxplLb69wa9QdiY7sW6Q6NAxeTBtocdqVJEfnrnPFFfK3k51krW4BF_Bhk6LG5EmazrrXDqYdxh1t6Te0Ubz7vC06lJqcaPdZvvV7p75s_VOr0JM-M8Dgq5w9bwc4ZswAlx1fp5_5jM1b1pLGcZpcyig2oIRpDiOqx6dNHzCxPV-fHnDW9IlZE02p5K9-Li4ghFI5VnsTEFUg1I2-TNYeTvay-MjK3KEsCvWxl_mRoiMa2clHm-JH0ocrzbKBqiYkdPDQNaee9eGAsrlaKBZNZSpQ"
	gotAdrr, err := url.QueryUnescape(encode)
	if err != nil {
		log.Fatalf("PIZDEC 2")
	}

	log.Println(gotAdrr)
	/*log.Print(len(encode))
	log.Println()

	encode2 := base64.URLEncoding.EncodeToString([]byte(addr))
	log.Print(len(encode2))
	log.Println()

	encode3 := url.QueryEscape(addr)
	log.Print(len(encode3))
	*/
	//if addr == gotAdrr {
	//	log.Println("OK")
	//}
}

/*func TestCompression(t *testing.T) {
	addr := "<ad><popunderAd><url><![CDATA[https://welloff-frame.com/c.n_RYiZPa2bJ-pdYejf0gx_NijjEk3lM-DnAompYq3_Us9tOuTvA-zxNyTzQAy_NCmDIExFO-GHIIwJMKD_gM5NZOWPI-1RZSGTQUw_ZWDXJYhZN-TbYcydZeD_Ug1hZiDjI-mlZmHnRor_PqTrIs0tN-DvIwlxMy0_JABBRCSDU-zFQGjHMIl_MK0LIM1NJ-mPRQ1RPST_kUwVMWzXU-0ZMajbZci_MeTfhgihM-DjAk4lOmW_VoipNqWrR-ktMuGvQwy_YyTzUA2BM-mDQE1FNGW_QIyJJKmLt-3NPOXPRQp_dSHTNUoVd-WXIYmZcam_VcmdPeWfh-0hdiHjAkl_Mm0nEolpM-krYsltMuk_Zw3xdy3zc-uBdCGDlE0_cG2HhI1JY-iL5MjNbO2_0QmRcSnTJ-pVZWDX0Yz_NaGb1cldc-0fZgkhRi0_kk5lbmlnR-xpZqVrRs6_MuGvRwxxc-3zlATBOCF_IExFSGTHE-mJcKnLNMs_POTPIQyRM-zTcU5VOWC_ZYyZca3bJ-jdPeXfJg0_Yiij1klld-Vn9oopZqS_ZsztSuWvQ-9xOyTzIA1_MCTDUEzFM-DHMI5JMKz_MM4NMOTPc-wRNSjTIU0_NWiXZYzZS-WbQcydPeX_dg3hdiyj5-0lamXnRoz_aqHrVsitL-mvNwvxbyS_ZAzBYCTD0-1FNGzHkIz_MKTLAMlNM-0PIQxRNSz_YUxVOWDXI-zZNajbcc4_JenfQg9hS-2jFkklOmH_NoTpVq1rl-wtNuEv0w5_ay1zRAOBe-kD1EDFZGF_JIyJbKmLd-vNNO0PhQ2_RSzTcU4VN-3XpYKZZa2_ZcUdTenfd-6hSimjJkZ_amGnlompT-nrVsvtbu1_FwsxQy1zY-5BeCTDNEh_SGkHRItJO-XLBMjNNO0_RQtRaSzTQ-uVOWXXFYp_ZaDbNc4dN-jfRgnhdim_JkflUm0nF-0pTqXrls0_Vumv8wxxN-WztAvBOCH_QE0FbGUHJ-YJZK2LhMr_UOWPwQyRW-WTgUwVSWj_dYvZSaGb5-YdUemflgT_Ui0j4kylR-WnxoppZqD_hs2tVuEvk-wxZyGz9Ak_RCEDNEyFY-3HMI1JbKU_JMaNWOmPR-ERcSmTJUQ_dWjXBYYZe-mbpcFdSez_BgfhSiTjI-zlbmEnhot_Qq2rss5tT-HvBwtxOyH_BAyBOCUDd-LFUGUHFIW_LKmLpMENV-nPVQwRdSF_JUjVXW2X8-4ZUaWbJc4_SeWfIgyhU-Uj9kPlNm0_4o1pTqWrF-QtXu0v9wp_by0ztAUBO-UDwE4FZGU_lImJTKVLA-wNSOjPZQR_eSGTdUlVN-HXpY4ZcaG_9cWdNeFfp-Jhdi2j1kZ_cmlnloLpR-Wr5sitSuE_xwQxeymz9-3BZCXDQEx_MGVHBIUJe-TLlMXNROD_YQzRUSmTl-FVcWDXZYN_QakbZcMdQ-nfVgvhTiX_ok1lNmUn4-wpdqVrIs4_MuEvJwKxS-lzpAkBeCj_JE1FeGkHx-KJaKWLZMx_TO3PVQwRU-2T1ULVTWG_9YxZZaEbt-rdde2flg6_RizjRkulc-Gn1otpbqX_dsptcukv1-PxZyjzdAp_OCWDFETFb-FHUIuJdKH_lMjNZOTPl-WRQSzTFUO_dWkXhYkZY-UbZcJdNey_5gFhdi1jV-KlWmjnlo1_cqHrFsltY-TvJwyxWyU_hA5BdCjDZ-PFVGVHFIO_SK1LRM2NQ-nPYQ1RaST_FUfVNWWXJ-zZbaEblc3_Lekf0guhV-DjFkSlUmW_ZoWpQqWr5-stNu2vdwZ_MykzJAYBT-DDVECFeGm_wI0JUKWLt-WNaOWP1Q0_TSXTVUhVY-UXZY0ZbaX_VcSdVeXfJ-jhaikjtkZ_OmXnNoYpb-0rJsCteuT_kwyxUyHzh-aBTCEDsE5_YGkHwI0JN-GLlMkNdOF_ZQ0RZSUT1-HVVWVXVYM_UaFbZcodW-HfhgThaiF_kk3lUmHnh-lpbqErssy_Nu1v8wzxY-0ztA1BNCV_pEwFMGmHs-zJVKmLRMI_VOnPVQWRe-lThUjVYWm_xYsZNaDbR-ZdSeFfAg3_VikjtktlM-2nRotpWqm_VsLtduWvt-WxUyEzRAR_UCHDNEkFU-3HlIIJbKl_FMwNWOnPd-LRSSlThUz_OWVXJYLZT-VbZc2dbeF_BgXhUinjA-ulam3nBoI_eqDrJsLtT-GvtwOxWyU_pAqBUC2Dl-yFVGUHVIv_dKELkM2Nb-EPZQnRdSz_dUkVaW2XJ-LZZaWbZcp_eenfNg5hU-ljdkIldmX_RoGpZqWrN-xtNukvZwk_UySz0AtB]]></url></popunderAd></ad>"

	encode := url.QueryEscape(addr)

	// 1. Base64
	b64 := base64.URLEncoding.EncodeToString([]byte(addr))

	// 2. Gzip + Base64
	gzipB64 := compressGzip(addr)

	// 3. Flate (более легкий алгоритм) + Base64
	flateB64 := compressFlate(addr)

	// 4. Своя компактная кодировка + Base64
	customB64 := customEncode(addr)

	// 5. Только замена частых паттернов
	patternReplaced := replacePatterns(addr)

	log.Printf("Original: %d chars", len(addr))
	log.Printf("urlEncode: %d chars", len(encode))
	log.Printf("Base64: %d chars", len(b64))
	log.Printf("Gzip+Base64: %d chars", len(gzipB64))
	log.Printf("Flate+Base64: %d chars", len(flateB64))
	log.Printf("Custom+Base64: %d chars", len(customB64))
	log.Printf("Pattern replaced: %d chars", len(patternReplaced))
}*/

func compressGzip(s string) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(s))
	gz.Close()
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

func compressFlate(s string) string {
	var buf bytes.Buffer
	fw, _ := flate.NewWriter(&buf, flate.BestCompression)
	fw.Write([]byte(s))
	fw.Close()
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

func customEncode(s string) string {
	// Сначала заменяем частые паттерны
	replaced := replacePatterns(s)
	// Потом сжимаем flate
	var buf bytes.Buffer
	fw, _ := flate.NewWriter(&buf, flate.BestCompression)
	fw.Write([]byte(replaced))
	fw.Close()
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

func replacePatterns(s string) string {
	// Анализируем ваш URL и заменяем частые паттерны
	replacements := map[string]string{
		"https://":             "h:",
		"expleates.pro":        "e",
		"thebeautyofgirls.com": "b",
		"sId=311053":           "s1",
		"&sId2=":               "&s2",
		"&t=":                  "&t",
		"%2F":                  "/", // декодируем некоторые URL encoding
		"%2B":                  "+",
		"%%%%":                 "%%", // упрощаем множественные %
	}

	result := s
	for old, new := range replacements {
		result = strings.ReplaceAll(result, old, new)
	}
	return result
}
