package main

import (
	"encoding/json"
	"html/template"
	"net/http"
)

var landingTmpl = template.Must(template.New("landing").Funcs(template.FuncMap{
	"json": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
}).Parse(`<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Name}} — Al Azhar Memorial Garden</title>
<meta name="description" content="{{.Description}}">
<meta name="robots" content="index, follow">
<meta name="author" content="Al Azhar Memorial Garden">
<link rel="canonical" href="{{.CanonicalURL}}">

<!-- Open Graph / Facebook / WhatsApp -->
<meta property="og:type" content="website">
<meta property="og:url" content="{{.CanonicalURL}}">
<meta property="og:title" content="{{.Name}} — Al Azhar Memorial Garden">
<meta property="og:description" content="{{.Description}}">
<meta property="og:image" content="{{.OGImage}}">
<meta property="og:image:width" content="600">
<meta property="og:image:height" content="315">
<meta property="og:image:alt" content="Al Azhar Memorial Garden">
<meta property="og:locale" content="id_ID">
<meta property="og:site_name" content="Al Azhar Memorial Garden">

<!-- Twitter Card -->
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Name}} — Al Azhar Memorial Garden">
<meta name="twitter:description" content="{{.Description}}">
<meta name="twitter:image" content="{{.OGImage}}">

<!-- Structured Data JSON-LD -->
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "WebPage",
  "name": {{.Name | json}},
  "description": {{.Description | json}},
  "url": {{.CanonicalURL | json}},
  "publisher": {
    "@type": "Organization",
    "name": "Al Azhar Memorial Garden",
    "url": "https://alazhar-agentic.farindra.com",
    "address": {
      "@type": "PostalAddress",
      "addressLocality": "Karawang",
      "addressRegion": "Jawa Barat",
      "addressCountry": "ID"
    }
  }
}
</script>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f8f9fa;min-height:100vh;display:flex;flex-direction:column;align-items:center;padding:32px 16px}
.logo-wrap{text-align:center;margin-bottom:24px}
.logo-wrap img{height:56px;object-fit:contain}
.brand{font-size:1.25rem;font-weight:700;color:#1a5f3f;margin-top:8px}
.sub{font-size:.85rem;color:#666;margin-top:4px}
.card{background:#fff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,.1);max-width:480px;width:100%}
.card-header{background:#1a5f3f;padding:20px 24px}
.card-header h1{font-size:1.15rem;color:#fff;font-weight:700}
.card-header p{font-size:.85rem;color:rgba(255,255,255,.85);margin-top:6px}
.card-body{padding:24px}
.hint{font-size:.85rem;color:#666;margin-bottom:16px}
.field{margin-bottom:14px}
label{display:block;font-size:.8rem;color:#444;margin-bottom:4px;font-weight:500}
input,select,textarea{width:100%;padding:10px 12px;border:1px solid #d1d5db;border-radius:8px;font-size:.9rem;outline:none;transition:border-color .2s}
input:focus,select:focus,textarea:focus{border-color:#1a5f3f;box-shadow:0 0 0 3px rgba(26,95,63,.1)}
textarea{resize:vertical;min-height:80px}
.custom-html{margin-bottom:14px}
.form-note{font-size:.78rem;color:#888;margin-top:14px;padding-top:14px;border-top:1px solid #f0f0f0}
.btn{width:100%;padding:14px;background:#1a5f3f;color:#fff;border:none;border-radius:10px;font-size:1rem;font-weight:600;cursor:pointer;display:flex;align-items:center;justify-content:center;gap:8px;margin-top:16px;transition:background .2s}
.btn:hover{background:#145032}
.btn:disabled{background:#9ca3af;cursor:not-allowed}
.alert-err{background:#fef2f2;border:1px solid #fca5a5;color:#dc2626;padding:10px 14px;border-radius:8px;font-size:.85rem;margin-bottom:14px}
.success{text-align:center;padding:40px 24px}
.success .ic{background:#dcfce7;border-radius:50%;width:64px;height:64px;display:flex;align-items:center;justify-content:center;margin:0 auto 16px}
.success h2{font-size:1.2rem;font-weight:700;color:#111}
.success p{font-size:.9rem;color:#666;margin-top:8px}
.success .redir{font-size:.75rem;color:#aaa;margin-top:12px}
footer{margin-top:24px;font-size:.75rem;color:#aaa;text-align:center}
</style>
{{range .Pixels}}{{if eq .Type "fb_pixel"}}
<script>!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version='2.0';n.queue=[];t=b.createElement(e);t.async=!0;t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window,document,'script','https://connect.facebook.net/en_US/fbevents.js');fbq('init','{{.ID}}');fbq('track','PageView');</script>
{{else if eq .Type "gtm"}}
<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','{{.ID}}');</script>
{{else if eq .Type "tiktok_pixel"}}
<script>!function(w,d,t){w.TiktokAnalyticsObject=t;var ttq=w[t]=w[t]||[];ttq.methods=["page","track","identify","instances","debug","on","off","once","ready","alias","group","enableCookie","disableCookie"],ttq.setAndDefer=function(t,e){t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}};for(var i=0;i<ttq.methods.length;i++)ttq.setAndDefer(ttq,ttq.methods[i]);ttq.instance=function(t){for(var e=ttq._i[t]||[],n=0;n<ttq.methods.length;n++)ttq.setAndDefer(e,ttq.methods[n]);return e},ttq.load=function(e,n){var i="https://analytics.tiktok.com/i18n/pixel/events.js";ttq._i=ttq._i||{},ttq._i[e]=[],ttq._i[e]._u=i,ttq._t=ttq._t||{},ttq._t[e]=+new Date,ttq._o=ttq._o||{},ttq._o[e]=n||{};var o=document.createElement("script");o.type="text/javascript",o.async=!0,o.src=i+"?sdkid="+e+"&lib="+t;var a=document.getElementsByTagName("script")[0];a.parentNode.insertBefore(o,a)};ttq.load('{{.ID}}');ttq.page();}(window,document,'ttq');</script>
{{else if eq .Type "ga4"}}
<script async src="https://www.googletagmanager.com/gtag/js?id={{.ID}}"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','{{.ID}}');</script>
{{end}}{{end}}
{{if .CustomScript}}<script>{{.CustomScript}}</script>{{end}}
</head>
<body>
{{range .Pixels}}{{if eq .Type "gtm"}}<noscript><iframe src="https://www.googletagmanager.com/ns.html?id={{.ID}}" height="0" width="0" style="display:none;visibility:hidden"></iframe></noscript>{{end}}{{end}}

<div class="logo-wrap">
  <img src="/static/al-azhar.png" alt="Al Azhar Memorial Garden" onerror="this.style.display='none'">
  <div class="brand">Al Azhar Memorial Garden</div>
  <div class="sub">Pemakaman Muslim No.1 di Indonesia</div>
</div>

<div class="card">
  <div class="card-header">
    <h1>{{.Name}}</h1>
    {{if .Description}}<p>{{.Description}}</p>{{end}}
  </div>

  <div id="form-wrap" class="card-body">
    <p class="hint">Isi formulir berikut dan tim kami akan menghubungi Anda segera.</p>

    {{if .CustomHTML}}<div class="custom-html">{{.CustomHTML}}</div>{{end}}

    <div id="alert" class="alert-err" style="display:none"></div>

    <form id="lp-form" novalidate>
      <div class="field">
        <label>Nama Lengkap <span style="color:red">*</span></label>
        <input type="text" id="f-name" placeholder="Nama lengkap Anda" required>
      </div>
      <div class="field">
        <label>No. WhatsApp / HP <span style="color:red">*</span></label>
        <input type="tel" id="f-phone" placeholder="08xxxxxxxxxx" required inputmode="tel">
      </div>
      <div class="field">
        <label>Email</label>
        <input type="email" id="f-email" placeholder="email@contoh.com" inputmode="email">
      </div>
      <div class="field">
        <label>Produk yang diminati</label>
        {{if .ProductIDs}}
        <select id="f-interest">
          <option value="">-- Pilih produk --</option>
          {{range .ProductIDs}}<option value="{{.}}">{{.}}</option>{{end}}
        </select>
        {{else}}
        <input type="text" id="f-interest" placeholder="Kavling standar, keluarga, premium...">
        {{end}}
      </div>
      <div class="field">
        <label>Pesan / Catatan</label>
        <textarea id="f-notes" placeholder="Tulis pertanyaan atau catatan Anda di sini..."></textarea>
      </div>
      {{if .FormNote}}<p class="form-note">{{.FormNote}}</p>{{end}}
      <button type="submit" class="btn" id="submit-btn">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" y1="2" x2="11" y2="13"></line><polygon points="22 2 15 22 11 13 2 9 22 2"></polygon></svg>
        Kirim Sekarang
      </button>
    </form>
  </div>

  <div id="success-wrap" class="success" style="display:none">
    <div class="ic">
      <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#16a34a" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>
    </div>
    <h2>Terima kasih!</h2>
    <p>Data Anda telah kami terima. Tim konsultan kami akan segera menghubungi Anda.</p>
    <p class="redir" id="redir-msg"></p>
    <p style="font-size:.75rem;color:#aaa;margin-top:8px">Jazakallahu Khayran 🤲</p>
  </div>
</div>

<footer>© Al Azhar Memorial Garden — Karawang, Jawa Barat</footer>

<script>
const SLUG = {{.Slug | json}};
const REDIRECT_TYPE = {{.RedirectType | json}};
const REDIRECT_URL = {{.RedirectURL | json}};

document.getElementById('lp-form').addEventListener('submit', async function(e) {
  e.preventDefault();
  const name = document.getElementById('f-name').value.trim();
  const phone = document.getElementById('f-phone').value.trim();
  const email = document.getElementById('f-email').value.trim();
  const interest = document.getElementById('f-interest').value.trim();
  const notes = document.getElementById('f-notes').value.trim();
  const alert = document.getElementById('alert');
  const btn = document.getElementById('submit-btn');

  alert.style.display = 'none';
  if (!name || !phone) {
    alert.textContent = 'Nama dan nomor HP wajib diisi';
    alert.style.display = 'block';
    return;
  }

  btn.disabled = true;
  btn.textContent = 'Mengirim...';

  try {
    const res = await fetch('/api/public/campaigns/' + SLUG + '/submit', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name, phone, email, interest, notes})
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Gagal mengirim');

    if (window.fbq) fbq('track', 'Lead');
    if (window.ttq) ttq.track('SubmitForm');
    if (window.gtag) gtag('event', 'generate_lead');

    document.getElementById('form-wrap').style.display = 'none';
    document.getElementById('success-wrap').style.display = 'block';

    const rt = data.redirect_type || REDIRECT_TYPE;
    const ru = data.redirect_url || REDIRECT_URL;
    const sp = data.salesman_phone;

    if (rt === 'wa' && sp) {
      document.getElementById('redir-msg').textContent = 'Mengarahkan ke WhatsApp...';
      const txt = encodeURIComponent("Assalamu'alaikum, saya tertarik dengan informasi dari Al Azhar Memorial Garden");
      setTimeout(() => { window.location.href = 'https://wa.me/' + sp + '?text=' + txt; }, 1500);
    } else if ((rt === 'website' || rt === 'custom_link') && ru) {
      document.getElementById('redir-msg').textContent = 'Mengarahkan...';
      setTimeout(() => { window.location.href = ru; }, 1500);
    }
  } catch(err) {
    alert.textContent = err.message;
    alert.style.display = 'block';
    btn.disabled = false;
    btn.textContent = 'Kirim Sekarang';
  }
});
</script>
</body>
</html>
`))

type landingData struct {
	*Campaign
	CustomScript template.JS
	CustomHTML   template.HTML
	CanonicalURL string
	OGImage      string
}

func handlePublicLandingPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	c, err := dbGetCampaignBySlug(r.Context(), slug)
	if err != nil || c.Status != "active" {
		http.NotFound(w, r)
		return
	}

	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	canonicalURL := scheme + "://" + r.Host + "/" + slug

	// OG image 600x315 dari campaign domain sendiri
	ogImage := scheme + "://" + r.Host + "/static/og-image.png"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	data := &landingData{
		Campaign:     c,
		CustomScript: template.JS(c.CustomScript),
		CustomHTML:   template.HTML(c.CustomHTML),
		CanonicalURL: canonicalURL,
		OGImage:      ogImage,
	}
	if err := landingTmpl.Execute(w, data); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}
