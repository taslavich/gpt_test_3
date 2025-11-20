package config

import (
	"log"
	"net/url"
	"testing"
)

/*
func TestMain(t *testing.T) {

		adid := "clickadilla.com=http://pop.zog.link/bid-request?token=h6dKfdh544FHD83,kadam.net=http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj"
		testAdm := MapStringToString{}
		err := testAdm.SetValue(adid)
		if err != nil {
			log.Fatalf("PIZDEC")
		}

		for k, v := range testAdm {
			log.Printf("--%s--", k)
			log.Printf("--%s--", v)
		}
	}
*/
func Test_Main(t *testing.T) {
	/*addr := "<ad><popunderAd><url><![CDATA[https://welloff-frame.com/c.n_RYiZPa2bJ-pdYejf0gx_NijjEk3lM-DnAompYq3_Us9tOuTvA-zxNyTzQAy_NCmDIExFO-GHIIwJMKD_gM5NZOWPI-1RZSGTQUw_ZWDXJYhZN-TbYcydZeD_Ug1hZiDjI-mlZmHnRor_PqTrIs0tN-DvIwlxMy0_JABBRCSDU-zFQGjHMIl_MK0LIM1NJ-mPRQ1RPST_kUwVMWzXU-0ZMajbZci_MeTfhgihM-DjAk4lOmW_VoipNqWrR-ktMuGvQwy_YyTzUA2BM-mDQE1FNGW_QIyJJKmLt-3NPOXPRQp_dSHTNUoVd-WXIYmZcam_VcmdPeWfh-0hdiHjAkl_Mm0nEolpM-krYsltMuk_Zw3xdy3zc-uBdCGDlE0_cG2HhI1JY-iL5MjNbO2_0QmRcSnTJ-pVZWDX0Yz_NaGb1cldc-0fZgkhRi0_kk5lbmlnR-xpZqVrRs6_MuGvRwxxc-3zlATBOCF_IExFSGTHE-mJcKnLNMs_POTPIQyRM-zTcU5VOWC_ZYyZca3bJ-jdPeXfJg0_Yiij1klld-Vn9oopZqS_ZsztSuWvQ-9xOyTzIA1_MCTDUEzFM-DHMI5JMKz_MM4NMOTPc-wRNSjTIU0_NWiXZYzZS-WbQcydPeX_dg3hdiyj5-0lamXnRoz_aqHrVsitL-mvNwvxbyS_ZAzBYCTD0-1FNGzHkIz_MKTLAMlNM-0PIQxRNSz_YUxVOWDXI-zZNajbcc4_JenfQg9hS-2jFkklOmH_NoTpVq1rl-wtNuEv0w5_ay1zRAOBe-kD1EDFZGF_JIyJbKmLd-vNNO0PhQ2_RSzTcU4VN-3XpYKZZa2_ZcUdTenfd-6hSimjJkZ_amGnlompT-nrVsvtbu1_FwsxQy1zY-5BeCTDNEh_SGkHRItJO-XLBMjNNO0_RQtRaSzTQ-uVOWXXFYp_ZaDbNc4dN-jfRgnhdim_JkflUm0nF-0pTqXrls0_Vumv8wxxN-WztAvBOCH_QE0FbGUHJ-YJZK2LhMr_UOWPwQyRW-WTgUwVSWj_dYvZSaGb5-YdUemflgT_Ui0j4kylR-WnxoppZqD_hs2tVuEvk-wxZyGz9Ak_RCEDNEyFY-3HMI1JbKU_JMaNWOmPR-ERcSmTJUQ_dWjXBYYZe-mbpcFdSez_BgfhSiTjI-zlbmEnhot_Qq2rss5tT-HvBwtxOyH_BAyBOCUDd-LFUGUHFIW_LKmLpMENV-nPVQwRdSF_JUjVXW2X8-4ZUaWbJc4_SeWfIgyhU-Uj9kPlNm0_4o1pTqWrF-QtXu0v9wp_by0ztAUBO-UDwE4FZGU_lImJTKVLA-wNSOjPZQR_eSGTdUlVN-HXpY4ZcaG_9cWdNeFfp-Jhdi2j1kZ_cmlnloLpR-Wr5sitSuE_xwQxeymz9-3BZCXDQEx_MGVHBIUJe-TLlMXNROD_YQzRUSmTl-FVcWDXZYN_QakbZcMdQ-nfVgvhTiX_ok1lNmUn4-wpdqVrIs4_MuEvJwKxS-lzpAkBeCj_JE1FeGkHx-KJaKWLZMx_TO3PVQwRU-2T1ULVTWG_9YxZZaEbt-rdde2flg6_RizjRkulc-Gn1otpbqX_dsptcukv1-PxZyjzdAp_OCWDFETFb-FHUIuJdKH_lMjNZOTPl-WRQSzTFUO_dWkXhYkZY-UbZcJdNey_5gFhdi1jV-KlWmjnlo1_cqHrFsltY-TvJwyxWyU_hA5BdCjDZ-PFVGVHFIO_SK1LRM2NQ-nPYQ1RaST_FUfVNWWXJ-zZbaEblc3_Lekf0guhV-DjFkSlUmW_ZoWpQqWr5-stNu2vdwZ_MykzJAYBT-DDVECFeGm_wI0JUKWLt-WNaOWP1Q0_TSXTVUhVY-UXZY0ZbaX_VcSdVeXfJ-jhaikjtkZ_OmXnNoYpb-0rJsCteuT_kwyxUyHzh-aBTCEDsE5_YGkHwI0JN-GLlMkNdOF_ZQ0RZSUT1-HVVWVXVYM_UaFbZcodW-HfhgThaiF_kk3lUmHnh-lpbqErssy_Nu1v8wzxY-0ztA1BNCV_pEwFMGmHs-zJVKmLRMI_VOnPVQWRe-lThUjVYWm_xYsZNaDbR-ZdSeFfAg3_VikjtktlM-2nRotpWqm_VsLtduWvt-WxUyEzRAR_UCHDNEkFU-3HlIIJbKl_FMwNWOnPd-LRSSlThUz_OWVXJYLZT-VbZc2dbeF_BgXhUinjA-ulam3nBoI_eqDrJsLtT-GvtwOxWyU_pAqBUC2Dl-yFVGUHVIv_dKELkM2Nb-EPZQnRdSz_dUkVaW2XJ-LZZaWbZcp_eenfNg5hU-ljdkIldmX_RoGpZqWrN-xtNukvZwk_UySz0AtB]]></url></popunderAd></ad>"
	encode := url.QueryEscape(addr)
	*/
	encode := "https%3A%2F%2Fkts.vasstycom.com%2Fin%2F2660%2F%3Fkatds_ep%3DaWhzWcYS2XH72EGLXQZ3UdPzn24TytsE1P1cQx-574ElqlhB6PYpll4BDe-2SKuYZ_L52wdKFZyMRpgv8kHtOnoIZFAb7m9hIWB8R1qmp7Pu3D7GCu2QQeKETU3-rl4gaeCza7cz5DKcvCQNhIrH7_vY2vradXnneXujpmqGGM7ki7CHdmph4gHdpO5OKIWNrdoojctnnNmpDmi740qTRpHaOrJxP5wZ-aX5gApUQihFP7bJ7jwy1EluREVmON7uuLk_NVgncXRAuXCOGwxPpcxvSN9rkYWtX1xCNkGnVk49zjxLUYxCRBPVHu9WFnfubykigQF2CLJ-dJGq6S3JXZQY4fbaVobA9OZERwXo0lIxGJbyhetvLmBJD078B1W_Jr48vGTfJDtH-PXoxD0CVAx68nwpSZbwRkPl0U4uXhiaBpCc7GzMtAhPnRuICjyU6mk0fMbGDTcqaJW6u0JLPwDJeIXyoUiVulZyD91cK_Jn5h9OtyATP876RuQ5FMV2X_ZchAQf7ccB6MhSKYmQcZqfVpJRcHQViYs3vXB3pZo0k5uChLVEEfCm6GvVu0-ZXV1OJuTdV8psaEZ1ci0_lWdJDZfXz7FiMSL81yt2xOXdgQAQZ6JQQe9k93AcxPZ0HPc8Dl2c8k9CxJF_4avK-uur66q4_3siCIuek_czgf1052QD1QP_dL9Ap_Y8kxNUO4F7L6wpA0O6Y06VJRWrtYAYpzSPXZI8Br71kAttpefPaIAxl-dQL4xFJY7zuWvku2GxFaFXYTigqFRDQjitEKDn1YXvKAieCg2X5kzsESmprxc9xjOT1S2uokdTb3wh7VSIahSm7IbgExpiplM6RcsT9y6TMSsWvdIF3yzQaFR8M2A6ViN40zXs_SvPAWAM8KoRMThEcTB9dr5QzTaGs3qkADrkutOClz3QCBeetxzjMBWFMlx9WSYNxdmZkJcwejv2AGFEUBwXA0P6NVmHyK9V1lb98ESzOggWQzCxUUde0KFYzuoPi33arZLG5R17aFEI6sV5HlECNh3xJ73j_gFQh1YsQphSvzoS9UWPWRF_z5An54xQBV5x4uXxq-XNZ4-EQNqWJD5blV7c8mnaQtYT7pyk58NlDYSkfZo9glnywfI8UViC2IlquV2g2ylqBU95M48A6ckLZroy_0DwR9BrsL2Q0HWQJj8JiHaST2NGIn-rvTaANgfFTFM7CixEzPQmDDftZJmqoboaNLhGAuEkWEduRHYhkHzaSedIB9qmO-tVYl-J2yLi8okItyBDeXCCdqVGjYi75Ma-UukWt4uhn4wj5eYCiVj86sbZ64kZSabdqFnzdgDXX63zRGz6eLrifWQLuyXFySOmOQHZUomqGABCmkp1p-bmKBH0FCBYn9kv6itU8aP1yo65z203rRi-y_zep8V84N-TR_EEXovmPxeZYbnW"
	gotAdrr, err := url.QueryUnescape(encode)
	if err != nil {
		log.Fatal(err.Error())
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
