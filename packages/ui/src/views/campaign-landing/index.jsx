import PropTypes from 'prop-types'
import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Alert, Box, Button, CircularProgress, Divider, MenuItem, Paper, Select, Stack, TextField, Typography } from '@mui/material'
import { IconSend, IconCheck } from '@tabler/icons-react'

const API = '/api/v1/crm'

function injectPixels(pixels) {
    if (!Array.isArray(pixels)) return
    pixels.forEach(({ type, id }) => {
        if (!id) return
        if (type === 'fb_pixel') {
            const s = document.createElement('script')
            s.innerHTML = `!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version='2.0';n.queue=[];t=b.createElement(e);t.async=!0;t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window,document,'script','https://connect.facebook.net/en_US/fbevents.js');fbq('init','${id}');fbq('track','PageView');`
            document.head.appendChild(s)
        } else if (type === 'gtm') {
            const s = document.createElement('script')
            s.innerHTML = `(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','${id}');`
            document.head.appendChild(s)
        } else if (type === 'tiktok_pixel') {
            const s = document.createElement('script')
            s.innerHTML = `!function(w,d,t){w.TiktokAnalyticsObject=t;var ttq=w[t]=w[t]||[];ttq.methods=["page","track","identify","instances","debug","on","off","once","ready","alias","group","enableCookie","disableCookie"],ttq.setAndDefer=function(t,e){t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}};for(var i=0;i<ttq.methods.length;i++)ttq.setAndDefer(ttq,ttq.methods[i]);ttq.instance=function(t){for(var e=ttq._i[t]||[],n=0;n<ttq.methods.length;n++)ttq.setAndDefer(e,ttq.methods[n]);return e},ttq.load=function(e,n){var i="https://analytics.tiktok.com/i18n/pixel/events.js";ttq._i=ttq._i||{},ttq._i[e]=[],ttq._i[e]._u=i,ttq._t=ttq._t||{},ttq._t[e]=+new Date,ttq._o=ttq._o||{},ttq._o[e]=n||{};var o=document.createElement("script");o.type="text/javascript",o.async=!0,o.src=i+"?sdkid="+e+"&lib="+t;var a=document.getElementsByTagName("script")[0];a.parentNode.insertBefore(o,a)};ttq.load('${id}');ttq.page();}(window,document,'ttq');`
            document.head.appendChild(s)
        } else if (type === 'ga4') {
            const s = document.createElement('script')
            s.async = true
            s.src = `https://www.googletagmanager.com/gtag/js?id=${id}`
            document.head.appendChild(s)
            const s2 = document.createElement('script')
            s2.innerHTML = `window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','${id}');`
            document.head.appendChild(s2)
        }
    })
}

function injectCustomCode(script) {
    if (script) {
        try {
            const s = document.createElement('script')
            s.innerHTML = script
            document.head.appendChild(s)
        } catch (e) {
            console.warn('Custom script error:', e)
        }
    }
    // custom html injected via dangerouslySetInnerHTML in the form area
}

function handleRedirect(result) {
    const { redirect_type, redirect_url, salesman_phone } = result
    if (redirect_type === 'wa' && salesman_phone) {
        const text = encodeURIComponent("Assalamu'alaikum, saya tertarik dengan informasi dari Al Azhar Memorial Garden")
        window.location.href = `https://wa.me/${salesman_phone}?text=${text}`
    } else if ((redirect_type === 'website' || redirect_type === 'custom_link') && redirect_url) {
        window.location.href = redirect_url
    }
    // jika tidak ada redirect, tetap di halaman sukses
}

function LandingPageContent({ campaign, isPreview }) {
    const [form, setForm] = useState({ name: '', phone: '', email: '', interest: '', notes: '' })
    const [submitting, setSubmitting] = useState(false)
    const [submitted, setSubmitted] = useState(false)
    const [error, setError] = useState(null)

    const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

    const handleSubmit = async (e) => {
        e.preventDefault()
        if (!form.name.trim() || !form.phone.trim()) {
            setError('Nama dan nomor HP wajib diisi')
            return
        }
        if (isPreview) {
            // di preview, hanya simulasi
            setSubmitted(true)
            return
        }
        setSubmitting(true)
        setError(null)
        try {
            const res = await fetch(`${API}/campaigns/public/${campaign.slug}/submit`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(form)
            })
            if (!res.ok) {
                const data = await res.json()
                throw new Error(data.error || 'Gagal mengirim')
            }
            const result = await res.json()
            if (window.fbq) window.fbq('track', 'Lead')
            if (window.ttq) window.ttq.track('SubmitForm')
            if (window.gtag) window.gtag('event', 'generate_lead')
            setSubmitted(true)
            // redirect setelah 1.5 detik biar user sempat lihat pesan sukses
            setTimeout(() => handleRedirect(result), 1500)
        } catch (err) {
            setError(err.message)
        } finally {
            setSubmitting(false)
        }
    }

    return (
        <Box
            sx={{
                minHeight: isPreview ? 'auto' : '100vh',
                bgcolor: '#f8f9fa',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                py: 4,
                px: 2
            }}
        >
            {/* Logo & header */}
            <Stack alignItems='center' spacing={1} sx={{ mb: 3, maxWidth: 480, width: '100%' }}>
                <Box
                    component='img'
                    src='/logo.png'
                    alt='AAMG'
                    sx={{ height: 56, objectFit: 'contain' }}
                    onError={(e) => {
                        e.target.style.display = 'none'
                    }}
                />
                <Typography variant='h5' fontWeight={700} textAlign='center' sx={{ color: '#1a5f3f' }}>
                    Al Azhar Memorial Garden
                </Typography>
                <Typography variant='body2' color='text.secondary' textAlign='center'>
                    Pemakaman Muslim No.1 di Indonesia
                </Typography>
            </Stack>

            <Paper elevation={3} sx={{ maxWidth: 480, width: '100%', borderRadius: 3, overflow: 'hidden' }}>
                {/* Campaign header */}
                <Box sx={{ bgcolor: '#1a5f3f', px: 3, py: 2.5 }}>
                    <Typography variant='h6' fontWeight={700} sx={{ color: '#fff' }}>
                        {campaign.name}
                    </Typography>
                    {campaign.description && (
                        <Typography variant='body2' sx={{ color: 'rgba(255,255,255,0.85)', mt: 0.5 }}>
                            {campaign.description}
                        </Typography>
                    )}
                </Box>

                {submitted ? (
                    <Stack alignItems='center' spacing={2} sx={{ p: 4 }}>
                        <Box sx={{ bgcolor: '#dcfce7', borderRadius: '50%', p: 2, display: 'flex' }}>
                            <IconCheck size={32} color='#16a34a' />
                        </Box>
                        <Typography variant='h6' fontWeight={700} textAlign='center'>
                            Terima kasih!
                        </Typography>
                        <Typography variant='body2' color='text.secondary' textAlign='center'>
                            Data Anda telah kami terima. Tim konsultan kami akan segera menghubungi Anda.
                        </Typography>
                        {campaign.redirect_type === 'wa' && !isPreview && (
                            <Typography variant='caption' color='text.disabled'>
                                Mengarahkan ke WhatsApp...
                            </Typography>
                        )}
                        <Typography variant='caption' color='text.disabled'>
                            Jazakallahu Khayran 🤲
                        </Typography>
                    </Stack>
                ) : (
                    <Box component='form' onSubmit={handleSubmit} sx={{ p: 3 }}>
                        <Typography variant='body2' color='text.secondary' sx={{ mb: 2 }}>
                            Isi formulir berikut dan tim kami akan menghubungi Anda segera.
                        </Typography>

                        {/* Custom HTML disisipkan di sini */}
                        {campaign.custom_html && <Box sx={{ mb: 2 }} dangerouslySetInnerHTML={{ __html: campaign.custom_html }} />}

                        {error && (
                            <Alert severity='error' sx={{ mb: 2 }}>
                                {error}
                            </Alert>
                        )}

                        <Stack spacing={2}>
                            <TextField size='small' fullWidth label='Nama Lengkap *' value={form.name} onChange={set('name')} required />
                            <TextField
                                size='small'
                                fullWidth
                                label='No. WhatsApp / HP *'
                                value={form.phone}
                                onChange={set('phone')}
                                placeholder='08xxxxxxxxxx'
                                required
                                inputMode='tel'
                            />
                            <TextField
                                size='small'
                                fullWidth
                                label='Email'
                                value={form.email}
                                onChange={set('email')}
                                type='email'
                                inputMode='email'
                            />

                            {campaign.product_ids?.length > 0 ? (
                                <Box>
                                    <Typography variant='caption' color='text.secondary' sx={{ mb: 0.5, display: 'block' }}>
                                        Produk yang diminati
                                    </Typography>
                                    <Select size='small' fullWidth displayEmpty value={form.interest} onChange={set('interest')}>
                                        <MenuItem value=''>-- Pilih produk --</MenuItem>
                                        {campaign.product_ids.map((p) => (
                                            <MenuItem key={p} value={p}>
                                                {p}
                                            </MenuItem>
                                        ))}
                                    </Select>
                                </Box>
                            ) : (
                                <TextField
                                    size='small'
                                    fullWidth
                                    label='Produk yang diminati'
                                    value={form.interest}
                                    onChange={set('interest')}
                                    placeholder='Kavling standar, keluarga, premium...'
                                />
                            )}

                            <TextField
                                size='small'
                                fullWidth
                                label='Pesan / Catatan'
                                value={form.notes}
                                onChange={set('notes')}
                                multiline
                                rows={3}
                                placeholder='Tulis pertanyaan atau catatan Anda di sini...'
                            />

                            {campaign.form_note && (
                                <>
                                    <Divider />
                                    <Typography variant='caption' color='text.secondary'>
                                        {campaign.form_note}
                                    </Typography>
                                </>
                            )}

                            <Button
                                type='submit'
                                variant='contained'
                                fullWidth
                                size='large'
                                disabled={submitting}
                                startIcon={submitting ? <CircularProgress size={18} color='inherit' /> : <IconSend size={18} />}
                                sx={{ bgcolor: '#1a5f3f', '&:hover': { bgcolor: '#145032' }, borderRadius: 2, py: 1.2 }}
                            >
                                {submitting ? 'Mengirim...' : isPreview ? 'Kirim (Preview)' : 'Kirim Sekarang'}
                            </Button>
                        </Stack>
                    </Box>
                )}
            </Paper>

            <Typography variant='caption' color='text.disabled' sx={{ mt: 3, textAlign: 'center' }}>
                © Al Azhar Memorial Garden — Karawang, Jawa Barat
            </Typography>
        </Box>
    )
}
LandingPageContent.propTypes = {
    campaign: PropTypes.object.isRequired,
    isPreview: PropTypes.bool
}

// Dipakai sebagai preview langsung (dari admin) ATAU sebagai halaman publik (/lp/:slug)
export default function CampaignLanding({ previewData }) {
    const params = useParams()
    const slug = previewData ? null : params?.slug

    const [campaign, setCampaign] = useState(previewData || null)
    const [loading, setLoading] = useState(!previewData)
    const [notFound, setNotFound] = useState(false)

    useEffect(() => {
        if (previewData) {
            setCampaign(previewData)
            return
        }
        fetch(`${API}/campaigns/public/${slug}`)
            .then((r) => {
                if (!r.ok) throw new Error('not found')
                return r.json()
            })
            .then((data) => {
                setCampaign(data)
                document.title = data.name + ' — Al Azhar Memorial Garden'
                injectPixels(data.pixels)
                injectCustomCode(data.custom_script)
            })
            .catch(() => setNotFound(true))
            .finally(() => setLoading(false))
    }, [slug, previewData])

    if (loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>
                <CircularProgress />
            </Box>
        )
    }

    if (notFound) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', p: 3 }}>
                <Stack alignItems='center' spacing={2}>
                    <Typography variant='h5' color='text.secondary'>
                        Halaman tidak ditemukan
                    </Typography>
                    <Typography variant='body2' color='text.disabled'>
                        Campaign ini mungkin sudah tidak aktif.
                    </Typography>
                </Stack>
            </Box>
        )
    }

    if (!campaign) return null

    return <LandingPageContent campaign={campaign} isPreview={Boolean(previewData)} />
}
CampaignLanding.propTypes = { previewData: PropTypes.object }
