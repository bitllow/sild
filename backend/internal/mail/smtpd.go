package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
)

// maxMessageBytes caps an accepted message; go-smtp advertises it as the SIZE
// extension and enforces it during DATA, bounding per-connection memory.
const maxMessageBytes = 26214400 // 25 MiB

// Handler ingests one parsed inbound email; called once per RCPT TO recipient.
type Handler func(ctx context.Context, in InboundEmail) error

// Serve runs the SMTP forwarding receiver (§6.2) on addr until ctx is cancelled.
// The wire protocol (EHLO, SIZE, dot-stuffing, timeouts, recipient limits) is
// handled by emersion/go-smtp — no auth/TLS, since the daemon sits behind the MX
// boundary (or is dev-local). We only collect the envelope, parse the MIME, and
// ingest once per recipient; the forwarding token resolves the tenant downstream.
func Serve(ctx context.Context, addr string, handle Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("sild-mail: SMTP receiver listening on %s", addr)
	return ServeListener(ctx, ln, handle)
}

// ServeListener runs the receiver over an existing listener until ctx is
// cancelled (Serve wraps it; tests use it to bind an ephemeral port).
func ServeListener(ctx context.Context, ln net.Listener, handle Handler) error {
	s := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) {
		return &session{ctx: ctx, handle: handle}, nil
	}))
	s.Domain = "sild-mail"
	s.MaxMessageBytes = maxMessageBytes
	s.MaxRecipients = 50
	s.ReadTimeout = 2 * time.Minute
	s.WriteTimeout = 2 * time.Minute
	go func() { <-ctx.Done(); _ = s.Close() }()
	return s.Serve(ln) // returns nil once Close() is called
}

// session collects the SMTP envelope and, on DATA, parses + ingests the message
// once per recipient.
type session struct {
	ctx    context.Context
	handle Handler
	from   string
	rcpts  []string
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error { s.from = from; return nil }
func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r) // bounded by MaxMessageBytes
	if err != nil {
		return err
	}
	for _, rcpt := range s.rcpts {
		in, perr := ParseInbound(raw, rcpt, s.from)
		if perr != nil {
			log.Printf("sild-mail: parse %s: %v", rcpt, perr)
			continue
		}
		if herr := s.handle(s.ctx, in); herr != nil {
			// The handler returns an error only for transient failures (see
			// domain.ForwardedMailHandler); ask the MTA to retry rather than
			// acknowledging, so a transient backend failure can't lose mail.
			log.Printf("sild-mail: ingest %s: %v", rcpt, herr)
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "temporary failure, please retry"}
		}
	}
	return nil
}

func (s *session) Reset()        { s.from, s.rcpts = "", nil }
func (s *session) Logout() error { return nil }

// ParseInbound parses raw RFC-5322 bytes into a normalized InboundEmail with any
// attachment bytes in memory (RawAttachments). recipient/envelopeFrom come from
// the SMTP envelope; the From header (preserved through forwarding) wins when
// present. Pure (no I/O) so it is unit-testable without a socket.
func ParseInbound(raw []byte, recipient, envelopeFrom string) (InboundEmail, error) {
	in := InboundEmail{Recipient: recipient, From: envelopeFrom, RawBody: raw, Headers: map[string]string{}}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		in.TextBody = string(raw) // not MIME — treat the whole payload as the body
		return in, nil
	}
	for k := range msg.Header {
		in.Headers[k] = msg.Header.Get(k)
	}
	dec := new(mime.WordDecoder)
	if subj, derr := dec.DecodeHeader(msg.Header.Get("Subject")); derr == nil {
		in.Subject = subj
	} else {
		in.Subject = msg.Header.Get("Subject")
	}
	if addr, perr := netmail.ParseAddress(msg.Header.Get("From")); perr == nil {
		in.From = addr.Address // original sender (forwarding preserves From)
	}
	text, atts := parseBody(msg.Body, msg.Header.Get("Content-Type"))
	in.TextBody = strings.TrimSpace(text)
	in.RawAttachments = atts
	return in, nil
}

// parseBody walks a (possibly multipart, possibly nested) MIME body, returning
// the best text part and the attachments. text/plain wins over text/html.
func parseBody(body io.Reader, contentType string) (string, []ParsedAttachment) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		b, _ := io.ReadAll(body)
		return string(b), nil
	}
	mr := multipart.NewReader(body, params["boundary"])
	var text, html string
	var atts []ParsedAttachment
	for {
		p, err := mr.NextPart()
		if err != nil {
			break // io.EOF or malformed tail — stop with what we have
		}
		partType, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		disp, _, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		data, _ := io.ReadAll(decodePart(p))
		switch {
		case disp == "attachment" || p.FileName() != "":
			atts = append(atts, ParsedAttachment{Filename: attachmentName(p), MimeType: partType, Content: data})
		case strings.HasPrefix(partType, "multipart/"):
			t, a := parseBody(bytes.NewReader(data), p.Header.Get("Content-Type"))
			if text == "" {
				text = t
			}
			atts = append(atts, a...)
		case partType == "text/plain":
			if text == "" {
				text = string(data)
			}
		case partType == "text/html":
			if html == "" {
				html = string(data)
			}
		}
	}
	if text == "" {
		text = html
	}
	return text, atts
}

// decodePart wraps a part reader to undo its transfer encoding.
func decodePart(p *multipart.Part) io.Reader {
	switch strings.ToLower(p.Header.Get("Content-Transfer-Encoding")) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, p)
	case "quoted-printable":
		return quotedprintable.NewReader(p)
	default:
		return p
	}
}

func attachmentName(p *multipart.Part) string {
	if fn := p.FileName(); fn != "" {
		return fn
	}
	return "attachment"
}
