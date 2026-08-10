package api

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/i18n"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
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
	scope, ok := apiutil.TranslationScope(c, h.svc)
	if !ok {
		return
	}
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
		if len(scope.Projects) > 0 && !slices.Contains(scope.Projects, v.Slug) {
			continue
		}
		rows = append(rows, translationProjectView(v))
	}
	apiutil.RespondPage(c, resourceTranslationProjects, store.SlicePage(rows, page, mapID))
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsManage, project, "") {
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsManage, project, "") {
		return
	}
	var req struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	err := h.svc.DeclareTranslationKey(c.Request.Context(), apiutil.Tenant(c), project, req.Key, req.Source)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) undeclareTranslationKey(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsManage, project, "") {
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsRead, project, locale) {
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsWrite, project, locale) {
		return
	}
	err := h.svc.PutTranslationOverride(c.Request.Context(), apiutil.Tenant(c),
		project, locale, c.Param("key"), req.Value)
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsWrite, project, locale) {
		return
	}
	err := h.svc.DeleteTranslationOverride(c.Request.Context(), apiutil.Tenant(c),
		project, locale, c.Param("key"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) publishTranslations(c *gin.Context) {
	project := translationProject(c)
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsPublish, project, "") {
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsPublish, project, "") {
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
	if !apiutil.AuthorizeTranslation(c, h.svc, policy.TranslationsRead, project, "") {
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
	if !apiutil.Authorize(c, policy.TranslationsFetch) {
		return
	}
	m, err := h.svc.TranslationManifest(c.Request.Context(), apiutil.Tenant(c), project)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	// Everything the body carries is in the validator, so a fallback changed on its
	// own still reaches a client holding the previous one.
	etag := fmt.Sprintf(`W/"%s-%s-%v"`, project, m.FallbackLocale, m.Locales)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, gin.H{
		"project": m.Project, "fallback_locale": m.FallbackLocale, "locales": m.Locales,
	})
}

func (h *Handler) translationBundle(c *gin.Context) {
	project := translationProject(c)
	locale := c.Query("locale")
	if !apiutil.Authorize(c, policy.TranslationsFetch) {
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

func (h *Handler) listTranslatorGrants(c *gin.Context) {
	if !apiutil.Authorize(c, policy.TranslationsManage) {
		return
	}
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceTranslatorGrants))
	if !ok {
		return
	}
	grants, err := h.svc.TranslatorGrants(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rows := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		rows = append(rows, map[string]any{
			"id": g.AdminUserID, "admin_user_id": g.AdminUserID,
			"email": g.Email, "name": g.Name,
			"projects": g.Projects, "locales": g.Locales,
		})
	}
	apiutil.RespondPage(c, resourceTranslatorGrants, store.SlicePage(descByID(rows), page, mapID))
}

func (h *Handler) setTranslatorGrant(c *gin.Context) {
	if !apiutil.Authorize(c, policy.TranslationsManage) {
		return
	}
	var req struct {
		Projects []string `json:"projects"`
		Locales  []string `json:"locales"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	err := h.svc.SetTranslatorScopes(c.Request.Context(), apiutil.Tenant(c),
		c.Param("id"), req.Projects, req.Locales)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
