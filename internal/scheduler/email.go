package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wneessen/go-mail"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/wecom"
)

type Mailer struct {
	cfg *config.SMTPConfig
}

func NewMailer(cfg *config.SMTPConfig) *Mailer { return &Mailer{cfg: cfg} }

func (m *Mailer) Enabled() bool {
	return m.cfg != nil && m.cfg.Enabled && m.cfg.Host != ""
}

// SendReport sends the report file to recipients. Returns an error when SMTP is
// disabled so the caller records a failed run.
func (m *Mailer) SendReport(recipients []string, subject, body, filePath string) error {
	if !m.Enabled() {
		return fmt.Errorf("SMTP 未配置，无法发送邮件")
	}
	if len(recipients) == 0 {
		return fmt.Errorf("收件人不能为空")
	}
	msg := mail.NewMsg()
	if err := msg.From(m.cfg.From); err != nil {
		return fmt.Errorf("发件人地址无效: %w", err)
	}
	if err := msg.To(recipients...); err != nil {
		return fmt.Errorf("收件人地址无效: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)
	msg.AttachFile(filePath, mail.WithFileName(filepath.Base(filePath)))

	opts := []mail.Option{
		mail.WithPort(m.cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(m.cfg.Username),
		mail.WithPassword(m.cfg.Password),
		mail.WithTimeout(30 * time.Second),
	}
	switch strings.ToLower(m.cfg.TLSMode) {
	case "ssl":
		opts = append(opts, mail.WithSSL())
	case "none":
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	client, err := mail.NewClient(m.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("创建邮件客户端失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}


// ---------- WeCom client adapter ----------

// WeComClient wraps the wecom package so the scheduler can be tested with a stub.
type WeComClient struct {
	client *wecom.Client
}

func NewWeComClient(cfg config.WeComConfig) *WeComClient {
	return &WeComClient{client: wecom.NewClient(cfg)}
}

func (c *WeComClient) Enabled() bool { return c.client.Enabled() }

func (c *WeComClient) SendText(ctx context.Context, to []string, content string) error {
	return c.client.SendText(ctx, to, content)
}

func (c *WeComClient) SendTextCard(ctx context.Context, to []string, card wecomTextCard) error {
	return c.client.SendTextCard(ctx, to, wecom.TemplateCard{
		Title: card.Title, Description: card.Description, URL: card.URL, BtnText: card.BtnText,
	})
}

// wecomTextCard mirrors the fields we send in a textcard message.
type wecomTextCard struct {
	Title       string
	Description string
	URL         string
	BtnText     string
}
