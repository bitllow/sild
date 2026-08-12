// Command sild-admin is the operator CLI: it creates tenants, operators and API
// keys. Everything goes through domain.Service, so the §1 invariants and password
// hashing hold exactly as they do over HTTP. It runs against the same database as
// the serving processes and never migrates (§4).
//
//	sild-admin tenant create --name "Acme" --admin-email a@acme.com --admin-name "A B"
//	sild-admin tenant list
//	sild-admin agent invite       --tenant <id> --email … --role owner|admin|agent
//	sild-admin agent set-password --tenant <id> --email …
//	sild-admin agent peer-access  --tenant <id> --email … --on
//	sild-admin apikey create --tenant <id> --label ci
//	sild-admin apikey revoke --tenant <id> --id <key id>
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/provision"
	"github.com/bitllow/sild/backend/internal/store/models"
	"golang.org/x/term"
)

const usage = `sild-admin — operator commands

  tenant create --name NAME --admin-email EMAIL [--admin-name "First Last"] [--password -]
  tenant list
  agent invite       --tenant ID --email EMAIL [--name "First Last"] [--role owner|admin|agent]
  agent set-password --tenant ID --email EMAIL [--password -]
  agent peer-access  --tenant ID --email EMAIL [--on|--off]
  apikey create --tenant ID [--label LABEL]
  apikey revoke --tenant ID --id KEY_ID

Passwords are read from a terminal prompt, or from stdin with --password -.
They are never taken from the command line, where they would land in shell
history and in the process list.
`

func main() {
	log.SetFlags(0)
	if len(os.Args) < 3 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1]+" "+os.Args[2], os.Args[3:]

	run, ok := commands[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	// No broker: these commands write through domain.Service and publish nothing
	// anyone is connected to receive, so the CLI must not need Redis to be up.
	c, err := di.New(di.WithoutRealtime())
	if err != nil {
		log.Fatalf("di: %v", err)
	}
	err = c.Invoke(func(svc *domain.Service) error { return run(context.Background(), svc, args) })
	if err != nil {
		log.Fatalf("sild-admin: %v", err)
	}
}

type command func(ctx context.Context, svc *domain.Service, args []string) error

var commands = map[string]command{
	"tenant create":      tenantCreate,
	"tenant list":        tenantList,
	"agent invite":       agentInvite,
	"agent set-password": agentSetPassword,
	"agent peer-access":  agentPeerAccess,
	"apikey create":      apikeyCreate,
	"apikey revoke":      apikeyRevoke,
}

func tenantCreate(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("tenant create", flag.ExitOnError)
	name := fs.String("name", "", "tenant name")
	email := fs.String("admin-email", "", "owner email")
	adminName := fs.String("admin-name", "", `owner display name, e.g. "Eva Marleen"`)
	pwSrc := fs.String("password", "", `"-" to read the owner password from stdin`)
	label := fs.String("label", "default", "label for the API key created with the tenant")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("name", name, "admin-email", email); err != nil {
		return err
	}
	// Prompt by default: a tenant with no password and no OIDC has no way in.
	password, err := readPassword(*pwSrc, "owner password: ")
	if err != nil {
		return err
	}
	res, err := provision.Tenant(ctx, svc, provision.TenantSpec{
		Name: *name, AdminEmail: *email, AdminName: *adminName,
		AdminPassword: password, APIKeyLabel: *label,
	})
	if err != nil {
		return err
	}
	fmt.Printf("tenant_id           %s\n", res.TenantID)
	fmt.Printf("api_key             %s\n", res.APIKey)
	fmt.Printf("forwarding_address  %s\n", res.ForwardingAddress)
	if password == "" {
		fmt.Printf("\nthe owner has no password yet: sild-admin agent set-password --tenant %s --email %s\n", res.TenantID, *email)
	}
	return nil
}

func tenantList(ctx context.Context, svc *domain.Service, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("tenant list takes no arguments")
	}
	tenants, err := svc.ListTenants(ctx)
	if err != nil {
		return err
	}
	for _, t := range tenants {
		fmt.Printf("%s  %s  %s\n", t.ID, t.CreatedAt.UTC().Format("2006-01-02"), t.Name)
	}
	return nil
}

func agentInvite(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("agent invite", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	email := fs.String("email", "", "operator email")
	name := fs.String("name", "", `display name, e.g. "Eva Marleen"`)
	role := fs.String("role", string(models.PlatformAgent), "owner|admin|agent|translator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant, "email", email); err != nil {
		return err
	}
	first, last := provision.SplitName(*name)
	admin, err := svc.InviteAgent(ctx, *tenant, *email, first, last, models.PlatformRole(*role), models.RoleScope{})
	if err != nil {
		return err
	}
	fmt.Printf("agent_id  %s\n", admin.ID)
	fmt.Printf("\nthey have no password yet: sild-admin agent set-password --tenant %s --email %s\n", *tenant, *email)
	return nil
}

func agentSetPassword(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("agent set-password", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	email := fs.String("email", "", "operator email")
	pwSrc := fs.String("password", "", `"-" to read the password from stdin`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant, "email", email); err != nil {
		return err
	}
	admin, err := lookup(ctx, svc, *tenant, *email)
	if err != nil {
		return err
	}
	password, err := readPassword(*pwSrc, "new password: ")
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New(`no password given: pass it on stdin with --password -`)
	}
	if err := svc.SetAdminPassword(ctx, *tenant, admin.ID, password); err != nil {
		return err
	}
	fmt.Printf("password set for %s\n", *email)
	return nil
}

func agentPeerAccess(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("agent peer-access", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	email := fs.String("email", "", "operator email")
	on := fs.Bool("on", false, "grant access to peer conversations")
	off := fs.Bool("off", false, "revoke access to peer conversations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *on == *off {
		return errors.New("pass exactly one of --on or --off")
	}
	if err := need("tenant", tenant, "email", email); err != nil {
		return err
	}
	admin, err := lookup(ctx, svc, *tenant, *email)
	if err != nil {
		return err
	}
	// Peer access is the agent role's own dimension, so this sets that role.
	if err := svc.SetRole(ctx, *tenant, admin.ID, models.PlatformAgent, models.RoleScope{Peer: *on}); err != nil {
		return err
	}
	fmt.Printf("peer access %v for %s\n", *on, *email)
	return nil
}

// splitList reads a comma-separated flag, dropping the blanks an empty flag or a
// trailing comma leaves behind.
func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func apikeyCreate(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("apikey create", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	label := fs.String("label", "default", "what this key is for")
	projects := fs.String("projects", "", "hold the key to these translation projects (comma-separated)")
	locales := fs.String("locales", "", "hold the key to these languages (comma-separated)")
	publish := fs.Bool("publish", false, "let the key cut a release")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant); err != nil {
		return err
	}
	scope := models.RoleScope{
		Projects: splitList(*projects), Locales: splitList(*locales), Publish: *publish,
	}
	key, rec, err := svc.CreateAPIKey(ctx, *tenant, *label, scope)
	if err != nil {
		return err
	}
	fmt.Printf("key_id   %s\n", rec.ID)
	fmt.Printf("api_key  %s   (shown once)\n", key)
	return nil
}

func apikeyRevoke(ctx context.Context, svc *domain.Service, args []string) error {
	fs := flag.NewFlagSet("apikey revoke", flag.ExitOnError)
	tenant := fs.String("tenant", "", "tenant id")
	id := fs.String("id", "", "key id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := need("tenant", tenant, "id", id); err != nil {
		return err
	}
	if err := svc.RevokeAPIKey(ctx, *tenant, *id); err != nil {
		return err
	}
	fmt.Printf("revoked %s\n", *id)
	return nil
}

// need reports the first missing required flag, named as the operator typed it.
func need(pairs ...any) error {
	var missing []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if *(pairs[i+1].(*string)) == "" {
			missing = append(missing, "--"+pairs[i].(string))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s %s required", strings.Join(missing, " and "), plural(len(missing)))
}

func plural(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func lookup(ctx context.Context, svc *domain.Service, tenant, email string) (*models.AdminUser, error) {
	admin, err := svc.FindAdminByEmail(ctx, tenant, email)
	if err != nil {
		return nil, fmt.Errorf("%s in tenant %s: %w", email, tenant, err)
	}
	return admin, nil
}

// readPassword resolves a password without ever reading it from argv: "-" reads
// one line from stdin, empty prompts on a terminal, and "" when there is neither.
func readPassword(src, prompt string) (string, error) {
	switch {
	case src == "-":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	case src != "":
		return "", errors.New(`--password only accepts "-" (read from stdin): a password on the command line lands in shell history and in the process list`)
	case term.IsTerminal(int(os.Stdin.Fd())):
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(b), nil
	default:
		return "", nil // no terminal to prompt on
	}
}
