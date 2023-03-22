package ja3

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

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

func NewClient(ctx context.Context, conn net.Conn, ja3Spec ClientHelloSpec, disHttp2 bool, addr string) (utlsConn *utls.UConn, err error) {
	defer func() {
		if err != nil {
			conn.Close()
			if utlsConn != nil {
				utlsConn.Close()
			}
		}
	}()
	var utlsSpec utls.ClientHelloSpec
	if !ja3Spec.IsSet() {
		if ja3Spec, err = CreateSpecWithId(HelloChrome_Auto); err != nil {
			return
		}
		utlsSpec = utls.ClientHelloSpec(ja3Spec)
	} else {
		utlsSpec = utls.ClientHelloSpec{
			CipherSuites:       ja3Spec.CipherSuites,
			CompressionMethods: ja3Spec.CompressionMethods,
			TLSVersMin:         ja3Spec.TLSVersMin,
			TLSVersMax:         ja3Spec.TLSVersMax,
			GetSessionID:       ja3Spec.GetSessionID,
		}
		utlsSpec.Extensions = make([]utls.TLSExtension, len(ja3Spec.Extensions))
		for i := 0; i < len(ja3Spec.Extensions); i++ {
			ext, ok := cloneExtension(ja3Spec.Extensions[i])
			if ok {
				utlsSpec.Extensions[i] = ext
			} else {
				utlsSpec.Extensions[i] = ja3Spec.Extensions[i]
			}
		}
	}
	if disHttp2 {
		utlsConn = utls.UClient(conn, &utls.Config{InsecureSkipVerify: true, ServerName: tools.GetServerName(addr), NextProtos: []string{"http/1.1"}}, utls.HelloCustom)
		for _, Extension := range utlsSpec.Extensions {
			alpns, ok := Extension.(*utls.ALPNExtension)
			if ok {
				if i := slices.Index(alpns.AlpnProtocols, "h2"); i != -1 {
					alpns.AlpnProtocols = slices.Delete(alpns.AlpnProtocols, i, i+1)
				}
				if i := slices.Index(alpns.AlpnProtocols, "http/1.1"); i == -1 {
					alpns.AlpnProtocols = append([]string{"http/1.1"}, alpns.AlpnProtocols...)
				}
				break
			}
		}
	} else {
		utlsConn = utls.UClient(conn, &utls.Config{
			InsecureSkipVerify:     true,
			InsecureSkipTimeVerify: true,
			ServerName:             tools.GetServerName(addr),
			NextProtos:             []string{"h2", "http/1.1"},
		},
			utls.HelloCustom,
		)
	}
	if err = utlsConn.ApplyPreset(&utlsSpec); err != nil {
		return
	}
	err = utlsConn.HandshakeContext(ctx)
	return
}

// https://www.iana.org/assignments/tls-extensiontype-values/tls-extensiontype-values.xhtml
func getExtensionWithId(extensionId uint16) utls.TLSExtension {
	switch extensionId {
	case 0:
		return &utls.SNIExtension{}
	case 5:
		return &utls.StatusRequestExtension{}
	case 13:
		return &utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
			utls.ECDSAWithP256AndSHA256,
			utls.ECDSAWithP384AndSHA384,
			utls.ECDSAWithP521AndSHA512,
			utls.PSSWithSHA256,
			utls.PSSWithSHA384,
			utls.PSSWithSHA512,
			utls.PKCS1WithSHA256,
			utls.PKCS1WithSHA384,
			utls.PKCS1WithSHA512,
			utls.ECDSAWithSHA1,
			utls.PKCS1WithSHA1,
		}}
	case 16:
		return &utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}}
	case 17:
		return &utls.StatusRequestV2Extension{}
	case 18:
		return &utls.SCTExtension{}
	case 21:
		return &utls.UtlsPaddingExtension{GetPaddingLen: utls.BoringPaddingStyle}
	case 23:
		return &utls.UtlsExtendedMasterSecretExtension{}
	case 24:
		return &utls.FakeTokenBindingExtension{}
	case 27:
		return &utls.UtlsCompressCertExtension{
			Algorithms: []utls.CertCompressionAlgo{utls.CertCompressionBrotli},
		}
	case 28:
		return &utls.FakeRecordSizeLimitExtension{} //Limit: 0x4001
	case 34:
		return &utls.FakeDelegatedCredentialsExtension{}
	case 35:
		return &utls.SessionTicketExtension{}
	case 41:
		return &utls.FakePreSharedKeyExtension{}
	case 44:
		return &utls.CookieExtension{}
	case 45:
		return &utls.PSKKeyExchangeModesExtension{Modes: []uint8{
			utls.PskModeDHE,
		}}
	case 50:
		return &utls.SignatureAlgorithmsCertExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
			utls.ECDSAWithP256AndSHA256,
			utls.ECDSAWithP384AndSHA384,
			utls.ECDSAWithP521AndSHA512,
			utls.PSSWithSHA256,
			utls.PSSWithSHA384,
			utls.PSSWithSHA512,
			utls.PKCS1WithSHA256,
			utls.PKCS1WithSHA384,
			utls.PKCS1WithSHA512,
			utls.ECDSAWithSHA1,
			utls.PKCS1WithSHA1,
		}}
	case 51:
		return &utls.KeyShareExtension{
			KeyShares: []utls.KeyShare{
				{Group: utls.CurveID(utls.GREASE_PLACEHOLDER), Data: []byte{0}},
				{Group: utls.X25519},
				{Group: utls.CurveP256},
			},
		}
	case 13172:
		return &utls.NPNExtension{}
	case 17513:
		return &utls.ApplicationSettingsExtension{SupportedProtocols: []string{"h2", "http/1.1"}}
	case 30031:
		return &utls.FakeChannelIDExtension{OldExtensionID: true} //FIXME
	case 30032:
		return &utls.FakeChannelIDExtension{} //FIXME
	case 65281:
		return &utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient}
	default:
		return nil
	}
}

func cloneExtension(extension utls.TLSExtension) (utls.TLSExtension, bool) {
	switch ext := extension.(type) {
	case *utls.KeyShareExtension:
		return &utls.KeyShareExtension{
			KeyShares: tools.CopySlices(ext.KeyShares),
		}, true
	case *utls.SNIExtension:
		return &utls.SNIExtension{
			ServerName: ext.ServerName,
		}, true
	case *utls.ALPNExtension:
		return &utls.ALPNExtension{
			AlpnProtocols: tools.CopySlices(ext.AlpnProtocols),
		}, true
	case *utls.SignatureAlgorithmsExtension:
		return &utls.SignatureAlgorithmsExtension{
			SupportedSignatureAlgorithms: tools.CopySlices(ext.SupportedSignatureAlgorithms),
		}, true
	case *utls.UtlsPaddingExtension:
		return &utls.UtlsPaddingExtension{
			PaddingLen:    ext.PaddingLen,
			WillPad:       ext.WillPad,
			GetPaddingLen: ext.GetPaddingLen,
		}, true
	case *utls.FakeTokenBindingExtension:
		return &utls.FakeTokenBindingExtension{
			MajorVersion:  ext.MajorVersion,
			MinorVersion:  ext.MinorVersion,
			KeyParameters: tools.CopySlices(ext.KeyParameters),
		}, true
	case *utls.UtlsCompressCertExtension:
		return &utls.UtlsCompressCertExtension{
			Algorithms: tools.CopySlices(ext.Algorithms),
		}, true
	case *utls.FakeRecordSizeLimitExtension:
		return &utls.FakeRecordSizeLimitExtension{
			Limit: ext.Limit,
		}, true
	case *utls.FakeDelegatedCredentialsExtension:
		return &utls.FakeDelegatedCredentialsExtension{
			SupportedSignatureAlgorithms: tools.CopySlices(ext.SupportedSignatureAlgorithms),
		}, true
	case *utls.SessionTicketExtension:
		var session *utls.ClientSessionState
		if ext.Session != nil {
			session = &utls.ClientSessionState{}
		}
		return &utls.SessionTicketExtension{
			Session: session,
		}, true
	case *utls.FakePreSharedKeyExtension:
		return &utls.FakePreSharedKeyExtension{
			PskIdentities: tools.CopySlices(ext.PskIdentities),
			PskBinders:    tools.CopySlicess(ext.PskBinders),
		}, true
	case *utls.CookieExtension:
		return &utls.CookieExtension{
			Cookie: tools.CopySlices(ext.Cookie),
		}, true
	case *utls.PSKKeyExchangeModesExtension:
		return &utls.PSKKeyExchangeModesExtension{
			Modes: tools.CopySlices(ext.Modes),
		}, true
	case *utls.SignatureAlgorithmsCertExtension:
		return &utls.SignatureAlgorithmsCertExtension{
			SupportedSignatureAlgorithms: tools.CopySlices(ext.SupportedSignatureAlgorithms),
		}, true
	case *utls.NPNExtension:
		return &utls.NPNExtension{
			NextProtos: tools.CopySlices(ext.NextProtos),
		}, true
	case *utls.ApplicationSettingsExtension:
		return &utls.ApplicationSettingsExtension{
			SupportedProtocols: tools.CopySlices(ext.SupportedProtocols),
		}, true
	case *utls.FakeChannelIDExtension:
		return &utls.FakeChannelIDExtension{
			OldExtensionID: ext.OldExtensionID,
		}, true
	case *utls.RenegotiationInfoExtension:
		return &utls.RenegotiationInfoExtension{
			Renegotiation: ext.Renegotiation,
		}, true
	// case *utls.StatusRequestExtension:
	// 	return ext, true
	// case *utls.StatusRequestV2Extension:
	// 	return 17, true
	// case *utls.SCTExtension:
	// 	return 18, true
	// case *utls.UtlsExtendedMasterSecretExtension:
	// 	return 23, true
	default:
		return nil, false
	}
}
func isGREASEUint16(v uint16) bool {
	// First byte is same as second byte
	// and lowest nibble is 0xa
	return ((v >> 8) == v&0xff) && v&0xf == 0xa
}

type ClientHelloSpec utls.ClientHelloSpec

func (obj ClientHelloSpec) IsSet() bool {
	if obj.CipherSuites == nil && obj.Extensions == nil && obj.CompressionMethods == nil && obj.TLSVersMax == 0 && obj.TLSVersMin == 0 {
		return false
	}
	return true
}

// ja3 clientHelloId 生成 clientHello
func CreateSpecWithId(ja3Id ClientHelloId) (clientHelloSpec ClientHelloSpec, err error) {
	spec, err := utls.UTLSIdToSpec(ja3Id)
	if err != nil {
		return clientHelloSpec, err
	}
	return ClientHelloSpec(spec), nil
}

// TLSVersion，Ciphers，Extensions，EllipticCurves，EllipticCurvePointFormats
func createTlsVersion(ver uint16, extensions []string) (tlsVersion uint16, tlsSuppor utls.TLSExtension, err error) {
	if slices.Index(extensions, "43") == -1 {
		err = errors.New("Extensions 缺少tlsVersion 扩展,检查ja3 字符串是否合法")
	}
	switch ver {
	case utls.VersionTLS13:
		tlsVersion = utls.VersionTLS13
		tlsSuppor = &utls.SupportedVersionsExtension{
			Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS13,
				utls.VersionTLS12,
				utls.VersionTLS11,
				utls.VersionTLS10,
			},
		}
	case utls.VersionTLS12:
		tlsVersion = utls.VersionTLS12
		tlsSuppor = &utls.SupportedVersionsExtension{
			Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS12,
				utls.VersionTLS11,
				utls.VersionTLS10,
			},
		}
	case utls.VersionTLS11:
		tlsVersion = utls.VersionTLS11
		tlsSuppor = &utls.SupportedVersionsExtension{
			Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS11,
				utls.VersionTLS10,
			},
		}
	default:
		err = errors.New("ja3Str 字符串中tls 版本错误")
	}
	return
}
func createCiphers(ciphers []string) ([]uint16, error) {
	cipherSuites := []uint16{utls.GREASE_PLACEHOLDER}
	for _, val := range ciphers {
		if n, err := strconv.ParseUint(val, 10, 16); err != nil {
			return nil, errors.New("ja3Str 字符串中cipherSuites错误")
		} else {
			cipherSuites = append(cipherSuites, uint16(n))
		}
	}
	return cipherSuites, nil
}
func createCurves(curves []string, extensions []string) (curvesExtension utls.TLSExtension, err error) {
	if slices.Index(extensions, "10") == -1 {
		err = errors.New("Extensions 缺少ellipticCurves 扩展,检查ja3 字符串是否合法")
	}
	curveIds := []utls.CurveID{utls.GREASE_PLACEHOLDER}
	for _, val := range curves {
		if n, err := strconv.ParseUint(val, 10, 16); err != nil {
			return nil, errors.New("ja3Str 字符串中cipherSuites错误")
		} else {
			curveIds = append(curveIds, utls.CurveID(uint16(n)))
		}
	}
	return &utls.SupportedCurvesExtension{Curves: curveIds}, nil
}
func createPointFormats(points []string, extensions []string) (curvesExtension utls.TLSExtension, err error) {
	if slices.Index(extensions, "11") == -1 {
		err = errors.New("Extensions 缺少pointFormats 扩展,检查ja3 字符串是否合法")
	}
	supportedPoints := []uint8{}
	for _, val := range points {
		if n, err := strconv.ParseUint(val, 10, 8); err != nil {
			return nil, errors.New("ja3Str 字符串中cipherSuites错误")
		} else {
			supportedPoints = append(supportedPoints, uint8(n))
		}
	}
	return &utls.SupportedPointsExtension{SupportedPoints: supportedPoints}, nil
}

func createExtensions(extensions []string, tlsExtension, curvesExtension, pointExtension utls.TLSExtension) ([]utls.TLSExtension, error) {
	allExtensions := []utls.TLSExtension{&utls.UtlsGREASEExtension{}}
	for _, extension := range extensions {
		var extensionId uint16
		if n, err := strconv.ParseUint(extension, 10, 16); err != nil {
			return nil, errors.New("ja3Str 字符串中extension错误,utls不支持的扩展: " + extension)
		} else {
			extensionId = uint16(n)
		}
		switch extensionId {
		case 10:
			allExtensions = append(allExtensions, curvesExtension)
		case 11:
			allExtensions = append(allExtensions, pointExtension)
		case 43:
			allExtensions = append(allExtensions, tlsExtension)
		default:
			ext := getExtensionWithId(extensionId)
			if ext == nil {
				if isGREASEUint16(extensionId) {
					allExtensions = append(allExtensions, &utls.UtlsGREASEExtension{})
				} else {
					allExtensions = append(allExtensions, &utls.GenericExtension{Id: extensionId})
				}
			} else {
				if ext == nil {
					return nil, errors.New("ja3Str 字符串中extension错误,utls不支持的扩展: " + extension)
				}
				if extensionId == 21 {
					allExtensions = append(allExtensions, &utls.UtlsGREASEExtension{})
				}
				allExtensions = append(allExtensions, ext)
			}
		}
	}
	return allExtensions, nil
}

// ja3 字符串中生成 clientHello
func CreateSpecWithStr(ja3Str string) (clientHelloSpec ClientHelloSpec, err error) {
	tokens := strings.Split(ja3Str, ",")
	if len(tokens) != 5 {
		return clientHelloSpec, errors.New("ja3Str 字符串格式不正确")
	}
	ver, err := strconv.ParseUint(tokens[0], 10, 16)
	if err != nil {
		return clientHelloSpec, errors.New("ja3Str 字符串中tls 版本错误")
	}
	ciphers := strings.Split(tokens[1], "-")
	extensions := strings.Split(tokens[2], "-")
	curves := strings.Split(tokens[3], "-")
	pointFormats := strings.Split(tokens[4], "-")
	tlsVersion, tlsExtension, err := createTlsVersion(uint16(ver), extensions)
	if err != nil {
		return clientHelloSpec, err
	}
	clientHelloSpec.TLSVersMin = utls.VersionTLS10
	clientHelloSpec.TLSVersMax = tlsVersion
	if clientHelloSpec.CipherSuites, err = createCiphers(ciphers); err != nil {
		return
	}
	curvesExtension, err := createCurves(curves, extensions)
	if err != nil {
		return clientHelloSpec, err
	}
	pointExtension, err := createPointFormats(pointFormats, extensions)
	if err != nil {
		return clientHelloSpec, err
	}
	clientHelloSpec.CompressionMethods = []byte{0x00}
	clientHelloSpec.GetSessionID = sha256.Sum256

	clientHelloSpec.Extensions, err = createExtensions(extensions, tlsExtension, curvesExtension, pointExtension)
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
