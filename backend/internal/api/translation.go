package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/i18n"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// translationProject is the addressed project, defaulting to the platform one.
// Whether it exists is the service's answer, not this function's.
func translationProject(c *gin.Context) string {
	project := c.Param("project")
	if project == "" {
		project = c.Query("project")
	}
	if project == "" {
		return i18n.PlatformProject
	}
	return project
}

// requireLocale reads the locale a write or a scoped read is addressed to.
// Never defaulted: authorizing "" and then resolving the tenant's fallback would
// admit a language a scoped translator does not hold.
func requireLocale(c *gin.Context, raw string) (string, bool) {
	locale := i18n.Normalize(raw)
	if locale == "" {
		httpx.BadRequest(c, "locale is required")
		return "", false
	}
	return locale, true
}

func (h *Handler) listTranslationProjects(c *gin.Context) {
	if !apiutil.Authorize(c, policy.TranslationsRead) {
		return
	}
	// A collection narrows rather than refusing: a translator granted one project
	// must land on it, and must not learn that the others exist.
	scope, narrows := apiutil.TranslationNarrowing(c, policy.TranslationsRead)
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceTranslationProjects))
	if !ok {
		return
	}
	vs, err := h.svc.ListTranslationProjects(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		if narrows && !models.Allows(scope.Projects, v.Slug) {
			continue
		}
		row := translationProjectView(v)
		if narrows {
			narrowToLanguages(row, scope.Locales)
		}
		rows = append(rows, row)
	}
	apiutil.RespondPage(c, resourceTranslationProjects, store.SlicePage(rows, page, mapID))
}

// narrowToLanguages leaves a scoped translator the languages they were granted:
// the screen they get must be the work they may actually do.
func narrowToLanguages(row map[string]any, granted []string) {
	keep := func(locales []string) []string {
		out := make([]string, 0, len(locales))
		for _, l := range locales {
			if models.Allows(granted, l) {
				out = append(out, l)
			}
		}
		return out
	}
	row["locales"] = keep(row["locales"].([]string))
	row["available_locales"] = keep(row["available_locales"].([]string))
	completion := map[string]int{}
	for locale, pct := range row["completion"].(map[string]int) {
		if models.Allows(granted, locale) {
			completion[locale] = pct
		}
	}
	row["completion"] = completion
}

func translationProjectView(v domain.TranslationProjectView) map[string]any {
	return map[string]any{
		"id": v.Slug, "slug": v.Slug, "name": v.Name, "platform": v.Platform,
		"fallback_locale": v.FallbackLocale, "auto_publish": v.AutoPublish,
		"locales": v.Locales, "available_locales": v.AvailableLocales,
		"current_version": v.CurrentVersion, "keys": v.Keys,
		"completion": v.Completion, "namespaces": v.Namespaces,
	}
}

func (h *Handler) createTranslationProject(c *gin.Context) {
	if !apiutil.Authorize(c, policy.TranslationsManage) {
		return
	}
	var req struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	slug := req.Slug
	if slug == "" {
		slug = req.ID
	}
	v, err := h.svc.CreateTranslationProject(c.Request.Context(), apiutil.Tenant(c), slug, req.Name)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, translationProjectView(v))
}

func (h *Handler) deleteTranslationProject(c *gin.Context) {
	if !apiutil.Authorize(c, policy.TranslationsManage) {
		return
	}
	err := h.svc.DeleteTranslationProject(c.Request.Context(), apiutil.Tenant(c), translationProject(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) saveTranslationProject(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsManage, project, "") {
		return
	}
	var req struct {
		Name           string   `json:"name"`
		FallbackLocale string   `json:"fallback_locale"`
		AutoPublish    bool     `json:"auto_publish"`
		Locales        []string `json:"locales"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	err := h.svc.SaveTranslationProject(c.Request.Context(), apiutil.Tenant(c),
		project, req.Name, req.FallbackLocale, req.AutoPublish, req.Locales)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) declareTranslationKey(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsManage, project, "") {
		return
	}
	var req struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		// Plurals is the source text per category, for a key addressed by a count.
		// Given instead of source, and what makes the key a plural.
		Plurals map[string]string `json:"plurals"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	err := h.svc.DeclareTranslationKey(c.Request.Context(), apiutil.Tenant(c),
		project, req.Key, req.Source, req.Plurals)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) undeclareTranslationKey(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsManage, project, "") {
		return
	}
	err := h.svc.UndeclareTranslationKey(c.Request.Context(), apiutil.Tenant(c), project, c.Param("key"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listTranslationKeys(c *gin.Context) {
	project := translationProject(c)
	locale, ok := requireLocale(c, c.Query("locale"))
	if !ok {
		return
	}
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsRead, project, locale) {
		return
	}
	page, ok := apiutil.PageParams(c, translationKeyPageDefaults())
	if !ok {
		return
	}
	res, err := h.svc.TranslationKeys(c.Request.Context(), apiutil.Tenant(c),
		project, locale, c.Query("state"), c.Query("namespace"), c.Query("q"), page)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(res.Items))
	for _, k := range res.Items {
		rows = append(rows, map[string]any{
			"key": k.Key, "namespace": k.Namespace,
			"source": k.Source, "value": k.Value, "state": k.State,
			"plural_base": k.PluralBase, "plural_category": k.PluralCategory,
			"placeholders": k.Placeholders,
		})
	}
	apiutil.RespondPage(c, resourceTranslationKeys, store.Page[map[string]any]{
		Items: rows, NextCursor: res.NextCursor, HasMore: res.HasMore,
	})
}

func (h *Handler) putTranslationKey(c *gin.Context) {
	project := translationProject(c)
	var req struct {
		Locale string `json:"locale"`
		Value  string `json:"value"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	locale, ok := requireLocale(c, req.Locale)
	if !ok {
		return
	}
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsWrite, project, locale) {
		return
	}
	err := h.svc.PutTranslationOverride(c.Request.Context(), apiutil.Tenant(c),
		project, locale, c.Param("key"), req.Value, apiutil.MayPublish(c, project))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) deleteTranslationKey(c *gin.Context) {
	project := translationProject(c)
	locale, ok := requireLocale(c, c.Query("locale"))
	if !ok {
		return
	}
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsWrite, project, locale) {
		return
	}
	err := h.svc.DeleteTranslationOverride(c.Request.Context(), apiutil.Tenant(c),
		project, locale, c.Param("key"), apiutil.MayPublish(c, project))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// importTranslations takes the file as the request body — a build pipeline pipes
// one in, and the raw-body shape is what the upload path already uses. Everything
// else is a query parameter.
func (h *Handler) importTranslations(c *gin.Context) {
	project := translationProject(c)
	locale, ok := requireLocale(c, c.Query("locale"))
	if !ok {
		return
	}
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsImport, project, locale) {
		return
	}
	format := i18n.Format(c.Query("format"))
	if format == "" {
		format = i18n.FormatJSON
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.BadRequest(c, "could not read the request body")
		return
	}
	if len(raw) == 0 {
		httpx.BadRequest(c, "the request body is the file to import, and it is empty")
		return
	}
	// Pushing the build's new source strings is what a pipeline is for, so a build
	// token declares keys; anyone else needs the capability that manages them. Off
	// unless asked, and only a source-language file may declare anything.
	createKeys := httpx.QueryBool(c, "create_keys")
	if createKeys && !apiutil.IsBuildToken(c) && !apiutil.Authorize(c, policy.TranslationsManage) {
		return
	}
	report, err := h.svc.ImportTranslations(c.Request.Context(), apiutil.Tenant(c),
		project, locale, format, raw, httpx.QueryBool(c, "dry_run"), createKeys,
		apiutil.MayPublish(c, project))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(report.Rows))
	for _, r := range report.Rows {
		rows = append(rows, map[string]any{
			"key": r.Key, "status": r.Status, "value": r.Value, "reason": r.Reason,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"project": report.Project, "locale": report.Locale, "format": report.Format,
		"dry_run": report.DryRun, "new": report.New, "changed": report.Changed,
		"skipped": report.Skipped, "keys_created": report.KeysMade, "rows": rows,
	})
}

func (h *Handler) exportTranslations(c *gin.Context) {
	project := translationProject(c)
	locale, ok := requireLocale(c, c.Query("locale"))
	if !ok {
		return
	}
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsExport, project, locale) {
		return
	}
	format := i18n.Format(c.Query("format"))
	if format == "" {
		format = i18n.FormatJSON
	}
	out, err := h.svc.ExportTranslations(c.Request.Context(), apiutil.Tenant(c), project, locale, format)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", i18n.Filename(format, project, locale)))
	c.Data(http.StatusOK, i18n.ContentType(format), out)
}

// draftTranslations is what publishing would change — the preview an owner
// approves, and the only thing a draft-only translator can see of their queue.
func (h *Handler) draftTranslations(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsRead, project, "") {
		return
	}
	// Narrowed before the diff is built, not after: a scoped translator's languages
	// are the only ones worth reading a bundle for.
	scope, narrows := apiutil.TranslationNarrowing(c, policy.TranslationsRead)
	var only []string
	if narrows && !models.Allows(scope.Locales, models.ScopeAll) {
		only = scope.Locales
	}
	diff, err := h.svc.TranslationDraftDiff(c.Request.Context(), apiutil.Tenant(c), project, only)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(diff.Rows))
	for _, r := range diff.Rows {
		rows = append(rows, map[string]any{
			"locale": r.Locale, "key": r.Key, "live": r.Live, "draft": r.Draft,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"project": diff.Project, "version": diff.Version,
		"next_version": diff.NextVersion, "rows": rows,
	})
}

func (h *Handler) publishTranslations(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsPublish, project, "") {
		return
	}
	rel, err := h.svc.PublishTranslations(c.Request.Context(), apiutil.Tenant(c), project, adminID(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	respondRelease(c, rel)
}

func (h *Handler) rollbackTranslations(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsPublish, project, "") {
		return
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		httpx.BadRequest(c, "version must be a number")
		return
	}
	rel, err := h.svc.RollbackTranslations(c.Request.Context(), apiutil.Tenant(c), project, version, adminID(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	respondRelease(c, rel)
}

func respondRelease(c *gin.Context, rel domain.TranslationReleaseView) {
	c.JSON(http.StatusCreated, gin.H{
		"version": rel.Version, "published_at": rel.PublishedAt, "locales": rel.Locales,
	})
}

func (h *Handler) listTranslationReleases(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsRead, project, "") {
		return
	}
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceTranslationReleases))
	if !ok {
		return
	}
	res, err := h.svc.TranslationReleases(c.Request.Context(), apiutil.Tenant(c), project, page)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(res.Items))
	for _, r := range res.Items {
		rows = append(rows, map[string]any{
			"version": r.Version, "published_at": r.PublishedAt, "published_by": r.PublishedBy,
		})
	}
	apiutil.RespondPage(c, resourceTranslationReleases, store.Page[map[string]any]{
		Items: rows, NextCursor: res.NextCursor, HasMore: res.HasMore,
	})
}

func (h *Handler) translationManifest(c *gin.Context) {
	project := translationProject(c)
	// Held to the project like every other translation read: a build token scoped to
	// one project must not learn what another publishes either.
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsFetch, project, "") {
		return
	}
	m, err := h.svc.TranslationManifest(c.Request.Context(), apiutil.Tenant(c), project)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	// Everything the body carries is in the validator, so a fallback changed on its
	// own still reaches a client holding the previous one.
	etag := fmt.Sprintf(`W/"%s-%s-%s-%v"`, project, i18n.CatalogVersion, m.FallbackLocale, m.Locales)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("ETag", etag)
	body := gin.H{
		"project": m.Project, "fallback_locale": m.FallbackLocale, "locales": m.Locales,
	}
	// Which SDK line these keys belong to, so upgrading is predictable. A tenant's
	// own project has no such thing: its keys are the tenant's.
	if project == i18n.PlatformProject {
		body["sdk_version"] = i18n.CatalogVersion
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) translationBundle(c *gin.Context) {
	project := translationProject(c)
	locale := i18n.Normalize(c.Query("locale"))
	if !apiutil.AuthorizeTranslation(c, policy.TranslationsFetch, project, locale) {
		return
	}
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil {
		httpx.BadRequest(c, "version must be a number")
		return
	}
	strs, err := h.svc.TranslationBundle(c.Request.Context(), apiutil.Tenant(c), project, locale, version)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	// A version's content never changes, so the client may hold it indefinitely.
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.JSON(http.StatusOK, gin.H{
		"project": project, "locale": i18n.Normalize(locale), "version": version, "strings": strs,
	})
}

// adminID is the operator a release is attributed to ("" for non-admin callers).
func adminID(c *gin.Context) string {
	if p := middleware.Get(c); p != nil {
		return p.AdminID
	}
	return ""
}

// translationKeyPageDefaults pages keys in key order — the list is a catalog.
func translationKeyPageDefaults() apiutil.PageDefaults {
	return apiutil.PageDefaults{
		Resource:   resourceTranslationKeys,
		Limit:      100,
		Sort:       store.SortID,
		Order:      store.OrderAsc,
		FixedOrder: true,
	}
}
