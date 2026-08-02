package admin

import (
	"bytes"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GMWalletApp/epusdt/config"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	appjwt "github.com/GMWalletApp/epusdt/util/jwt"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

type LoginRequest struct {
	Username string `json:"username" validate:"required" example:"admin"`
	TOTPCode string `json:"totp_code" validate:"required" example:"123456"`
}

type LoginResponse struct {
	Token                 string `json:"token"`
	Username              string `json:"username"`
	UserID                uint64 `json:"user_id"`
	AuthMethod            string `json:"auth_method"`
	SecuritySetupRequired bool   `json:"security_setup_required"`
}

type MeResponse struct {
	mdb.AdminUser
	PasskeyCount int `json:"passkey_count"`
}

type SensitiveAuthRequest struct {
	TOTPCode string `json:"totp_code" validate:"required"`
}

type PasskeyRegisterStartRequest struct {
	Username string `json:"username" validate:"required"`
	TOTPCode string `json:"totp_code" validate:"required"`
	Name     string `json:"name" validate:"required"`
}

type PasskeyFinishRequest struct {
	Username    string          `json:"username"`
	ChallengeID string          `json:"challenge_id" validate:"required"`
	Credential  json.RawMessage `json:"credential" validate:"required"`
	Name        string          `json:"name"`
}

type PasskeyLoginStartRequest struct {
	Username string `json:"username" validate:"required"`
}

type webAuthnUser struct {
	user        *mdb.AdminUser
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	id := make([]byte, 8)
	binary.BigEndian.PutUint64(id, u.user.ID)
	return id
}
func (u *webAuthnUser) WebAuthnName() string        { return u.user.Username }
func (u *webAuthnUser) WebAuthnDisplayName() string { return u.user.Username }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func newWebAuthn() (*webauthn.WebAuthn, error) {
	origins := config.GetAdminWebAuthnOrigins()
	if len(origins) == 0 {
		return nil, errors.New("admin WebAuthn origin is not configured")
	}
	parsed, err := url.Parse(origins[0])
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("invalid admin WebAuthn origin")
	}
	rpID := config.GetAdminWebAuthnRPID()
	if rpID == "" {
		rpID = parsed.Hostname()
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "GMPay 管理后台",
		RPID:          rpID,
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 60 * time.Second},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 60 * time.Second},
		},
	})
}

func loadWebAuthnUser(user *mdb.AdminUser) (*webAuthnUser, error) {
	credentials, err := data.AdminWebAuthnCredentials(user.ID)
	if err != nil {
		return nil, err
	}
	return &webAuthnUser{user: user, credentials: credentials}, nil
}

func clientIP(ctx echo.Context) string {
	remote, _, err := net.SplitHostPort(strings.TrimSpace(ctx.Request().RemoteAddr))
	if err != nil {
		remote = strings.TrimSpace(ctx.Request().RemoteAddr)
	}
	ip := net.ParseIP(remote)
	// Only trust forwarded headers when the direct peer is a local/private
	// reverse proxy. Public clients cannot forge their way around IP limits.
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		for _, raw := range strings.Split(ctx.Request().Header.Get(echo.HeaderXForwardedFor), ",") {
			if forwarded := net.ParseIP(strings.TrimSpace(raw)); forwarded != nil {
				return forwarded.String()
			}
		}
	}
	if ip != nil {
		return ip.String()
	}
	return remote
}

func (c *BaseAdminController) loginLocked(ctx echo.Context, until time.Time) error {
	retry := int(time.Until(until).Seconds())
	if retry < 1 {
		retry = 1
	}
	ctx.Response().Header().Set("Retry-After", strconv.Itoa(retry))
	return ctx.JSON(http.StatusTooManyRequests, map[string]interface{}{
		"status_code": 10047,
		"message":     constant.Errno[10047],
		"data":        map[string]int{"retry_after": retry},
		"request_id":  ctx.Request().Header.Get(echo.HeaderXRequestID),
	})
}

func verifyTOTP(user *mdb.AdminUser, code string) bool {
	if user == nil || strings.TrimSpace(user.TOTPSecret) == "" || len(strings.TrimSpace(code)) != 6 {
		return false
	}
	stored := strings.TrimSpace(user.TOTPSecret)
	secret := strings.ToUpper(strings.ReplaceAll(stored, " ", ""))
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret); err != nil {
		return false
	}
	return totp.Validate(strings.TrimSpace(code), secret)
}

func validateSensitiveAuth(user *mdb.AdminUser, code string) error {
	if user == nil || strings.TrimSpace(user.TOTPSecret) == "" || !verifyTOTP(user, code) {
		return constant.AdminTOTPInvalidErr
	}
	return nil
}

func issueAdminToken(user *mdb.AdminUser, method string) (*LoginResponse, error) {
	token, err := appjwt.SignWithVersion(user.ID, user.Username, user.AuthVersion)
	if err != nil {
		return nil, err
	}
	return &LoginResponse{Token: token, Username: user.Username, UserID: user.ID, AuthMethod: method, SecuritySetupRequired: false}, nil
}

func (c *BaseAdminController) Login(ctx echo.Context) error {
	req := new(LoginRequest)
	if err := ctx.Bind(req); err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if err := c.ValidateStruct(ctx, req); err != nil || len(req.Username) > 64 || len(strings.TrimSpace(req.TOTPCode)) != 6 {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}

	now := time.Now().UTC()
	keys := data.AdminLoginThrottleKeys(req.Username, clientIP(ctx))
	lockedUntil, err := data.AdminLoginLockedUntil(keys, now)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	if lockedUntil.After(now) {
		return c.loginLocked(ctx, lockedUntil)
	}

	user, err := data.GetAdminUserByUsername(req.Username)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	if user.ID == 0 || user.Status != mdb.AdminUserStatusEnable || strings.TrimSpace(user.TOTPSecret) == "" || !verifyTOTP(user, req.TOTPCode) {
		until, recordErr := data.RecordAdminLoginFailure(keys, now)
		if recordErr != nil {
			return c.FailJson(ctx, recordErr)
		}
		if until.After(now) {
			return c.loginLocked(ctx, until)
		}
		return c.FailJson(ctx, constant.AdminTOTPInvalidErr)
	}
	if err := data.ClearAdminLoginFailures(keys); err != nil {
		return c.FailJson(ctx, err)
	}
	if err := data.TouchAdminUserLastLogin(user.ID); err != nil {
		return c.FailJson(ctx, err)
	}
	result, err := issueAdminToken(user, "totp")
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, result)
}

func (c *BaseAdminController) PasskeyLoginStart(ctx echo.Context) error {
	req := new(PasskeyLoginStartRequest)
	if err := ctx.Bind(req); err != nil || c.ValidateStruct(ctx, req) != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	keys := data.AdminLoginThrottleKeys(req.Username, clientIP(ctx))
	if until, err := data.AdminLoginLockedUntil(keys, time.Now().UTC()); err != nil {
		return c.FailJson(ctx, err)
	} else if until.After(time.Now().UTC()) {
		return c.loginLocked(ctx, until)
	}
	user, err := data.GetAdminUserByUsername(req.Username)
	if err != nil || user.ID == 0 || user.Status != mdb.AdminUserStatusEnable {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	waUser, err := loadWebAuthnUser(user)
	if err != nil || len(waUser.credentials) == 0 {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	wa, err := newWebAuthn()
	if err != nil {
		return c.FailJson(ctx, err)
	}
	assertion, session, err := wa.BeginLogin(waUser)
	if err != nil {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	payload, _ := json.Marshal(session)
	challengeID, err := data.CreateAdminAuthChallenge(user.ID, mdb.AdminAuthChallengePasskeyLogin, string(payload), clientIP(ctx))
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, map[string]interface{}{"challenge_id": challengeID, "publicKey": assertion.Response})
}

func requestWithCredential(req *http.Request, raw json.RawMessage) *http.Request {
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(raw))
	clone.ContentLength = int64(len(raw))
	clone.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return clone
}

func (c *BaseAdminController) PasskeyLoginFinish(ctx echo.Context) error {
	req := new(PasskeyFinishRequest)
	if err := ctx.Bind(req); err != nil || c.ValidateStruct(ctx, req) != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	keys := data.AdminLoginThrottleKeys(req.Username, clientIP(ctx))
	now := time.Now().UTC()
	if until, err := data.AdminLoginLockedUntil(keys, now); err != nil {
		return c.FailJson(ctx, err)
	} else if until.After(now) {
		return c.loginLocked(ctx, until)
	}
	user, err := data.GetAdminUserByUsername(req.Username)
	if err != nil || user.ID == 0 || user.Status != mdb.AdminUserStatusEnable {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	challenge, err := data.ConsumeAdminAuthChallenge(req.ChallengeID, mdb.AdminAuthChallengePasskeyLogin, clientIP(ctx), user.ID)
	if err != nil {
		return c.FailJson(ctx, constant.AdminAuthChallengeErr)
	}
	var session webauthn.SessionData
	if json.Unmarshal([]byte(challenge.Payload), &session) != nil {
		return c.FailJson(ctx, constant.AdminAuthChallengeErr)
	}
	waUser, err := loadWebAuthnUser(user)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	wa, err := newWebAuthn()
	if err != nil {
		return c.FailJson(ctx, err)
	}
	credential, err := wa.FinishLogin(waUser, session, requestWithCredential(ctx.Request(), req.Credential))
	if err != nil {
		until, recordErr := data.RecordAdminLoginFailure(keys, now)
		if recordErr != nil {
			return c.FailJson(ctx, recordErr)
		}
		if until.After(now) {
			return c.loginLocked(ctx, until)
		}
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	if err := data.TouchAdminPasskey(user.ID, credential); err != nil {
		return c.FailJson(ctx, err)
	}
	_ = data.ClearAdminLoginFailures(keys)
	_ = data.TouchAdminUserLastLogin(user.ID)
	result, err := issueAdminToken(user, "passkey")
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, result)
}

func (c *BaseAdminController) PasskeyRegisterStart(ctx echo.Context) error {
	req := new(PasskeyRegisterStartRequest)
	if err := ctx.Bind(req); err != nil || c.ValidateStruct(ctx, req) != nil || len(strings.TrimSpace(req.Name)) > 80 {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	keys := data.AdminLoginThrottleKeys(req.Username, clientIP(ctx))
	now := time.Now().UTC()
	if until, err := data.AdminLoginLockedUntil(keys, now); err != nil {
		return c.FailJson(ctx, err)
	} else if until.After(now) {
		return c.loginLocked(ctx, until)
	}
	user, err := data.GetAdminUserByUsername(req.Username)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	if user.ID == 0 || user.Status != mdb.AdminUserStatusEnable || validateSensitiveAuth(user, req.TOTPCode) != nil {
		until, recordErr := data.RecordAdminLoginFailure(keys, now)
		if recordErr != nil {
			return c.FailJson(ctx, recordErr)
		}
		if until.After(now) {
			return c.loginLocked(ctx, until)
		}
		return c.FailJson(ctx, constant.AdminTOTPInvalidErr)
	}
	waUser, err := loadWebAuthnUser(user)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	wa, err := newWebAuthn()
	if err != nil {
		return c.FailJson(ctx, err)
	}
	creation, session, err := wa.BeginRegistration(waUser, webauthn.WithExclusions(webauthn.Credentials(waUser.credentials).CredentialDescriptors()))
	if err != nil {
		return c.FailJson(ctx, err)
	}
	payload, _ := json.Marshal(map[string]interface{}{"session": session, "name": strings.TrimSpace(req.Name)})
	challengeID, err := data.CreateAdminAuthChallenge(user.ID, mdb.AdminAuthChallengePasskeyRegister, string(payload), clientIP(ctx))
	if err != nil {
		return c.FailJson(ctx, err)
	}
	_ = data.ClearAdminLoginFailures(keys)
	return c.SucJson(ctx, map[string]interface{}{"challenge_id": challengeID, "publicKey": creation.Response})
}

func (c *BaseAdminController) PasskeyRegisterFinish(ctx echo.Context) error {
	req := new(PasskeyFinishRequest)
	if err := ctx.Bind(req); err != nil || c.ValidateStruct(ctx, req) != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	user, err := data.GetAdminUserByUsername(req.Username)
	if err != nil || user.ID == 0 || user.Status != mdb.AdminUserStatusEnable || strings.TrimSpace(user.TOTPSecret) == "" {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	challenge, err := data.ConsumeAdminAuthChallenge(req.ChallengeID, mdb.AdminAuthChallengePasskeyRegister, clientIP(ctx), user.ID)
	if err != nil {
		return c.FailJson(ctx, constant.AdminAuthChallengeErr)
	}
	var payload struct {
		Session webauthn.SessionData `json:"session"`
		Name    string               `json:"name"`
	}
	if json.Unmarshal([]byte(challenge.Payload), &payload) != nil {
		return c.FailJson(ctx, constant.AdminAuthChallengeErr)
	}
	waUser, err := loadWebAuthnUser(user)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	wa, err := newWebAuthn()
	if err != nil {
		return c.FailJson(ctx, err)
	}
	credential, err := wa.FinishRegistration(waUser, payload.Session, requestWithCredential(ctx.Request(), req.Credential))
	if err != nil {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	if err := data.SaveAdminPasskey(user.ID, payload.Name, credential); err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, nil)
}

func (c *BaseAdminController) ListPasskeys(ctx echo.Context) error {
	rows, err := data.ListAdminPasskeys(currentAdminUserID(ctx))
	if err != nil {
		return c.FailJson(ctx, err)
	}
	type item struct {
		ID         uint64      `json:"id"`
		Name       string      `json:"name"`
		CreatedAt  interface{} `json:"created_at"`
		LastUsedAt *time.Time  `json:"last_used_at"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		out = append(out, item{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt})
	}
	return c.SucJson(ctx, out)
}

func (c *BaseAdminController) DeletePasskey(ctx echo.Context) error {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	uid := currentAdminUserID(ctx)
	user, err := data.GetAdminUserByID(uid)
	if err != nil || user.ID == 0 {
		return c.FailJson(ctx, constant.AdminUnauthorizedErr)
	}
	req := new(SensitiveAuthRequest)
	if err := ctx.Bind(req); err != nil || c.ValidateStruct(ctx, req) != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := validateSensitiveAuth(user, req.TOTPCode); err != nil {
		return c.FailJson(ctx, err)
	}
	if err := data.DeleteAdminPasskey(uid, id); err != nil {
		return c.FailJson(ctx, constant.AdminPasskeyUnavailableErr)
	}
	if err := data.IncrementAdminAuthVersion(uid); err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, nil, "passkey deleted; sign in again")
}

func (c *BaseAdminController) Logout(ctx echo.Context) error {
	uid := currentAdminUserID(ctx)
	if uid != 0 {
		if err := data.IncrementAdminAuthVersion(uid); err != nil {
			return c.FailJson(ctx, err)
		}
	}
	return c.SucJson(ctx, nil)
}

func (c *BaseAdminController) Me(ctx echo.Context) error {
	uid := currentAdminUserID(ctx)
	if uid == 0 {
		return c.FailJson(ctx, constant.AdminUnauthorizedErr)
	}
	user, err := data.GetAdminUserByID(uid)
	if err != nil || user.ID == 0 {
		return c.FailJson(ctx, constant.AdminUserNotFoundErr)
	}
	passkeys, err := data.ListAdminPasskeys(uid)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, MeResponse{AdminUser: *user, PasskeyCount: len(passkeys)})
}
