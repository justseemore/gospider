package ja3

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"gitee.com/baixudong/gospider/tools"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/exp/slices"
)

type ClientHelloId = utls.ClientHelloID

var (
	HelloFirefox_Auto = utls.HelloFirefox_Auto
	HelloFirefox_55   = utls.HelloFirefox_55
	HelloFirefox_56   = utls.HelloFirefox_56
	HelloFirefox_63   = utls.HelloFirefox_63
	HelloFirefox_65   = utls.HelloFirefox_65
	HelloFirefox_99   = utls.HelloFirefox_99
	HelloFirefox_102  = utls.HelloFirefox_102
	HelloFirefox_105  = utls.HelloFirefox_105

	HelloChrome_Auto        = utls.HelloChrome_Auto
	HelloChrome_58          = utls.HelloChrome_58
	HelloChrome_62          = utls.HelloChrome_62
	HelloChrome_70          = utls.HelloChrome_70
	HelloChrome_72          = utls.HelloChrome_72
	HelloChrome_83          = utls.HelloChrome_83
	HelloChrome_87          = utls.HelloChrome_87
	HelloChrome_96          = utls.HelloChrome_96
	HelloChrome_100         = utls.HelloChrome_100
	HelloChrome_102         = utls.HelloChrome_102
	HelloChrome_106_Shuffle = utls.HelloChrome_106_Shuffle

	HelloIOS_Auto = utls.HelloIOS_Auto
	HelloIOS_11_1 = utls.HelloIOS_11_1
	HelloIOS_12_1 = utls.HelloIOS_12_1
	HelloIOS_13   = utls.HelloIOS_13
	HelloIOS_14   = utls.HelloIOS_14

	HelloAndroid_11_OkHttp = utls.HelloAndroid_11_OkHttp

	HelloEdge_Auto = utls.HelloEdge_Auto
	HelloEdge_85   = utls.HelloEdge_85
	HelloEdge_106  = utls.HelloEdge_106

	HelloSafari_Auto = utls.HelloSafari_Auto
	HelloSafari_16_0 = utls.HelloSafari_16_0

	Hello360_Auto = utls.Hello360_Auto
	Hello360_7_5  = utls.Hello360_7_5
	Hello360_11_0 = utls.Hello360_11_0

	HelloQQ_Auto = utls.HelloQQ_Auto
	HelloQQ_11_1 = utls.HelloQQ_11_1
)

func Ja3DialContext(ctx context.Context, conn net.Conn, ja3Id ClientHelloId, h2 bool, serverName string) (utlsConn *utls.UConn, err error) {
	defer func() {
		if err != nil {
			conn.Close()
			if utlsConn != nil {
				utlsConn.Close()
			}
		}
	}()
	var spec utls.ClientHelloSpec
	if spec, err = utls.UTLSIdToSpec(ja3Id); err != nil {
		return
	}
	if !h2 {
		for i := 0; i < len(spec.Extensions); i++ {
			if extension, ok := spec.Extensions[i].(*utls.ALPNExtension); ok {
				alns := []string{}
				for _, aln := range extension.AlpnProtocols {
					if aln != "h2" {
						alns = append(alns, aln)
					}
				}
				extension.AlpnProtocols = alns
			}
		}
	}
	utlsConn = utls.UClient(conn, &utls.Config{InsecureSkipVerify: true, ServerName: serverName}, utls.HelloCustom)
	if err = utlsConn.ApplyPreset(&spec); err != nil {
		return
	}
	if err = utlsConn.HandshakeContext(ctx); err != nil {
		return
	}
	return
}

type ClientHello struct {
	ServerName        string
	SupportedProtos   []string      //列出客户端支持的应用协议。[h2 http/1.1]
	SupportedPoints   []uint8       //列出了客户端支持的点格式[0]
	SupportedCurves   []tls.CurveID //列出了客户端支持的椭圆曲线。 [CurveID(2570) X25519 CurveP256 CurveP384]
	SupportedVersions []uint16      //列出了客户端支持的TLS版本。[2570 772 771]

	CipherSuites     []uint16              //客户端支持的密码套件 [14906 4865 4866 4867 49195 49199 49196 49200 52393 52392 49171 49172 156 157 47 53]
	SignatureSchemes []tls.SignatureScheme //列出了客户端愿意验证的签名和散列方案[ECDSAWithP256AndSHA256 PSSWithSHA256 PKCS1WithSHA256 ECDSAWithP384AndSHA384 PSSWithSHA384 PKCS1WithSHA384 PSSWithSHA512 PKCS1WithSHA512]
}

var ja3Db = map[string]string{
	"488e60390163900f2f8017b1a529fb71": "Firefox",
	"dd9ff85d6cf3dda49608196462e523d4": "Firefox",
	"60b4c43df20ec1e3b12338f36a3bb2ac": "Firefox",
	"479c1e69605c4fdfbf1f20ce7a94e2c5": "Firefox",
	"e2e2f297474a013695275e093bae0765": "Firefox",
	"017a847bdd23336891f3177b12eeeb11": "Chrome",
	"984835adab330788cc23dbca98bd0729": "Chrome",
	"4ebfa507a641965bc7b681da6b3eef0f": "Chrome",
	"fd3e1e3ffa5a9cbfe286d570172b6d11": "Chrome",
	"fa02513ba7bc98366d780cc67511bb11": "iOS",
	"c1d4aef20ee8c9373aaae75fe5e18fe3": "iOS",
	"3be6eceab63f23928637fba0c9d60f72": "iOS",
	"88fcc8e1c8b91a7ae3fd17d62aff1317": "iOS",
	"811b5bb18faa39a6927a393b4a084249": "Android",
	"97ab38518a3b2569ebfa4997a6aba778": "Safari",
	"6f1c1aa30872aff0d2ed761f219f6a99": "360Browser",
}

func newClientHello(chi *tls.ClientHelloInfo) ClientHello {
	if chi.SupportedCurves[0] != tls.X25519 {
		chi.SupportedCurves = chi.SupportedCurves[1:]
	}
	if chi.SupportedVersions[0] != 772 {
		chi.SupportedVersions = chi.SupportedVersions[1:]
	}
	if chi.CipherSuites[0] != 4865 {
		chi.CipherSuites = chi.CipherSuites[1:]
	}
	var CipherSuites []uint16
	for _, CipherSuite := range chi.CipherSuites {
		if slices.Index(CipherSuites, CipherSuite) == -1 {
			CipherSuites = append(CipherSuites, CipherSuite)
		}
	}
	chi.CipherSuites = CipherSuites
	return ClientHello{
		ServerName:        chi.ServerName,
		CipherSuites:      chi.CipherSuites,
		SupportedCurves:   chi.SupportedCurves,
		SupportedPoints:   chi.SupportedPoints,
		SignatureSchemes:  chi.SignatureSchemes,
		SupportedProtos:   chi.SupportedProtos,
		SupportedVersions: chi.SupportedVersions,
	}
}

type Ja3ContextData struct {
	ClientHello ClientHello `json:"clientHello"`
	Init        bool        `json:"init"`
}

func (obj Ja3ContextData) Md5() string {
	var md5Str string
	for _, val := range obj.ClientHello.SupportedProtos {
		md5Str += val
	}
	for _, val := range obj.ClientHello.SupportedPoints {
		md5Str += fmt.Sprintf("%d", val)
	}
	for _, val := range obj.ClientHello.SupportedCurves {
		md5Str += val.String()
	}
	for _, val := range obj.ClientHello.SupportedVersions {
		md5Str += fmt.Sprintf("%d", val)
	}
	for _, val := range obj.ClientHello.CipherSuites {
		md5Str += fmt.Sprintf("%d", val)
	}
	for _, val := range obj.ClientHello.SignatureSchemes {
		md5Str += val.String()
	}
	return tools.Hex(tools.Md5(md5Str))
}
func (obj Ja3ContextData) Verify() (string, bool) {
	ja3Name, ja3Ok := ja3Db[obj.Md5()]
	return ja3Name, ja3Ok
}

const keyPrincipalID = "Ja3ContextData"

func ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, keyPrincipalID, &Ja3ContextData{})
}
func GetConfigForClient(chi *tls.ClientHelloInfo) (*tls.Config, error) {
	chi.Context().Value(keyPrincipalID).(*Ja3ContextData).ClientHello = newClientHello(chi)
	return nil, nil
}
func GetRequestJa3Data(r *http.Request) *Ja3ContextData {
	return r.Context().Value(keyPrincipalID).(*Ja3ContextData)
}
