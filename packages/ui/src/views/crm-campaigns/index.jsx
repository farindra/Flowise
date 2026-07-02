import PropTypes from 'prop-types'
import { useEffect, useState, useCallback, useRef } from 'react'
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    Grid,
    IconButton,
    InputLabel,
    MenuItem,
    Paper,
    Select,
    Snackbar,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Tooltip,
    Typography
} from '@mui/material'
import {
    IconRefresh,
    IconPlus,
    IconEdit,
    IconTrash,
    IconCopy,
    IconEye,
    IconArrowLeft,
    IconX,
    IconBrandFacebook,
    IconBrandGoogleAnalytics,
    IconBrandTiktok,
    IconBrandWhatsapp,
    IconWorld,
    IconLink
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'
import CampaignLanding from '@/views/campaign-landing'

const API = '/api/v1/crm'
const CAMPAIGN_DOMAIN = import.meta.env.VITE_CAMPAIGN_DOMAIN || 'https://alazhar-campaign.farindra.com'

const PIXEL_TYPES = [
    { value: 'fb_pixel', label: 'Facebook Pixel', icon: <IconBrandFacebook size={14} />, placeholder: 'ID Pixel, cth: 123456789012345' },
    {
        value: 'gtm',
        label: 'Google Tag Manager',
        icon: <IconBrandGoogleAnalytics size={14} />,
        placeholder: 'Container ID, cth: GTM-XXXXXX'
    },
    { value: 'tiktok_pixel', label: 'TikTok Pixel', icon: <IconBrandTiktok size={14} />, placeholder: 'Pixel ID, cth: CXXXXXXXXXXXXXXXXX' },
    {
        value: 'ga4',
        label: 'Google Analytics 4',
        icon: <IconBrandGoogleAnalytics size={14} />,
        placeholder: 'Measurement ID, cth: G-XXXXXXXXXX'
    }
]

const REDIRECT_TYPES = [
    { value: 'wa', label: 'WhatsApp Salesman', icon: <IconBrandWhatsapp size={14} />, desc: 'Redirect ke WA salesman yang di-assign' },
    { value: 'website', label: 'Website', icon: <IconWorld size={14} />, desc: 'Redirect ke URL website yang ditentukan' },
    { value: 'custom_link', label: 'Custom Link', icon: <IconLink size={14} />, desc: 'Redirect ke link custom bebas' }
]

const EMPTY_FORM = {
    name: '',
    slug: '',
    description: '',
    product_ids_raw: '',
    pixels: [],
    form_note: '',
    custom_script: '',
    custom_html: '',
    redirect_type: 'wa',
    redirect_url: '',
    status: 'active'
}

function getLandingURL(slug) {
    return `${CAMPAIGN_DOMAIN}/${slug}`
}

function slugify(s) {
    return s
        .toLowerCase()
        .replace(/[àáâãäå]/g, 'a')
        .replace(/[èéêë]/g, 'e')
        .replace(/[ìíîï]/g, 'i')
        .replace(/[òóôõö]/g, 'o')
        .replace(/[ùúûü]/g, 'u')
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '')
}

function PixelEditor({ pixels, onChange }) {
    const add = () => onChange([...pixels, { type: 'fb_pixel', id: '' }])
    const remove = (i) => onChange(pixels.filter((_, idx) => idx !== i))
    const setField = (i, k) => (e) => {
        const next = [...pixels]
        next[i] = { ...next[i], [k]: e.target.value }
        onChange(next)
    }
    return (
        <Stack spacing={1}>
            {pixels.map((px, i) => {
                const meta = PIXEL_TYPES.find((t) => t.value === px.type) || PIXEL_TYPES[0]
                return (
                    <Stack key={i} direction='row' spacing={1} alignItems='center'>
                        <FormControl size='small' sx={{ minWidth: 170 }}>
                            <Select value={px.type} onChange={setField(i, 'type')}>
                                {PIXEL_TYPES.map((t) => (
                                    <MenuItem key={t.value} value={t.value}>
                                        <Stack direction='row' spacing={0.5} alignItems='center'>
                                            {t.icon}
                                            <Typography variant='caption'>{t.label}</Typography>
                                        </Stack>
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                        <TextField
                            size='small'
                            sx={{ flex: 1 }}
                            placeholder={meta.placeholder}
                            value={px.id}
                            onChange={setField(i, 'id')}
                        />
                        <IconButton size='small' color='error' onClick={() => remove(i)}>
                            <IconX size={14} />
                        </IconButton>
                    </Stack>
                )
            })}
            <Box>
                <Chip size='small' label='+ Tambah Pixel' clickable onClick={add} icon={<IconPlus size={12} />} variant='outlined' />
            </Box>
        </Stack>
    )
}
PixelEditor.propTypes = { pixels: PropTypes.array.isRequired, onChange: PropTypes.func.isRequired }

function CampaignDialog({ open, campaign, onClose, onSaved }) {
    const [form, setForm] = useState(EMPTY_FORM)
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState(null)
    const [slugEdited, setSlugEdited] = useState(false)
    const [slugStatus, setSlugStatus] = useState(null) // null | 'checking' | 'ok' | 'taken'
    const slugTimer = useRef(null)

    useEffect(() => {
        if (campaign) {
            setForm({ ...EMPTY_FORM, ...campaign, product_ids_raw: (campaign.product_ids || []).join('\n') })
            setSlugEdited(true)
            setSlugStatus(null)
        } else {
            setForm(EMPTY_FORM)
            setSlugEdited(false)
            setSlugStatus(null)
        }
        setError(null)
    }, [campaign, open])

    const checkSlug = useCallback(async (slug, excludeId) => {
        if (!slug) return
        setSlugStatus('checking')
        try {
            const qs = excludeId ? `?slug=${slug}&exclude_id=${excludeId}` : `?slug=${slug}`
            const res = await fetch(`${API}/campaigns/check-slug${qs}`)
            const data = await res.json()
            setSlugStatus(data.exists ? 'taken' : 'ok')
        } catch {
            setSlugStatus(null)
        }
    }, [])

    const set = (k) => (e) => {
        const val = e.target.value
        setForm((f) => {
            const next = { ...f, [k]: val }
            if (k === 'name' && !slugEdited) {
                next.slug = slugify(val)
                // debounce slug check
                clearTimeout(slugTimer.current)
                slugTimer.current = setTimeout(() => checkSlug(next.slug, campaign?.id), 600)
            }
            return next
        })
        if (k === 'slug') {
            setSlugEdited(true)
            clearTimeout(slugTimer.current)
            slugTimer.current = setTimeout(() => checkSlug(val, campaign?.id), 500)
        }
    }

    const save = async () => {
        if (!form.name.trim()) {
            setError('Nama wajib diisi')
            return
        }
        if (!form.slug.trim()) {
            setError('Slug wajib diisi')
            return
        }
        setSaving(true)
        setError(null)
        try {
            // auto-resolve duplicate slug
            let slug = form.slug
            let counter = 2
            let exists = slugStatus === 'taken'
            if (!exists && slugStatus !== 'ok') {
                const res = await fetch(`${API}/campaigns/check-slug?slug=${slug}${campaign?.id ? '&exclude_id=' + campaign.id : ''}`)
                exists = (await res.json()).exists
            }
            while (exists) {
                const candidate = `${form.slug}-${counter}`
                const res = await fetch(`${API}/campaigns/check-slug?slug=${candidate}${campaign?.id ? '&exclude_id=' + campaign.id : ''}`)
                exists = (await res.json()).exists
                if (!exists) slug = candidate
                counter++
                if (counter > 99) break
            }

            const payload = {
                ...form,
                slug,
                product_ids: form.product_ids_raw
                    .split('\n')
                    .map((s) => s.trim())
                    .filter(Boolean),
                pixels: form.pixels.filter((p) => p.id.trim())
            }
            delete payload.product_ids_raw

            const url = campaign ? `${API}/campaigns/${campaign.id}` : `${API}/campaigns`
            const res = await fetch(url, {
                method: campaign ? 'PUT' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
            if (!res.ok) {
                const d = await res.json()
                throw new Error(d.error || 'Gagal menyimpan')
            }
            onSaved()
            onClose()
        } catch (e) {
            setError(e.message)
        } finally {
            setSaving(false)
        }
    }

    const slugHelperText = () => {
        if (!form.slug) return ''
        const url = `${getLandingURL(form.slug)}`
        if (slugStatus === 'checking') return `Mengecek... — ${url}`
        if (slugStatus === 'taken') return `⚠ Slug sudah dipakai — akan otomatis ditambah angka saat simpan`
        if (slugStatus === 'ok') return `✓ Tersedia — ${url}`
        return url
    }

    const slugColor = slugStatus === 'taken' ? 'warning' : slugStatus === 'ok' ? 'success' : 'primary'

    return (
        <Dialog open={open} onClose={onClose} maxWidth='md' fullWidth>
            <DialogTitle>{campaign ? 'Edit Campaign' : 'Buat Campaign Baru'}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ mt: 1 }}>
                    {error && <Alert severity='error'>{error}</Alert>}

                    {/* Basic info */}
                    <Grid container spacing={1.5}>
                        <Grid item xs={12}>
                            <TextField size='small' fullWidth label='Nama Campaign *' value={form.name} onChange={set('name')} />
                        </Grid>
                        <Grid item xs={12}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Slug *'
                                value={form.slug}
                                onChange={set('slug')}
                                color={slugColor}
                                helperText={slugHelperText()}
                                inputProps={{ style: { fontFamily: 'monospace' } }}
                            />
                        </Grid>
                        <Grid item xs={12}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Deskripsi'
                                value={form.description}
                                onChange={set('description')}
                                multiline
                                rows={2}
                            />
                        </Grid>
                    </Grid>

                    <Divider />
                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        ANALYTICS PIXELS
                    </Typography>
                    <PixelEditor pixels={form.pixels} onChange={(p) => setForm((f) => ({ ...f, pixels: p }))} />

                    <Divider />
                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        KONFIGURASI FORM
                    </Typography>
                    <TextField
                        size='small'
                        fullWidth
                        label='Pilihan Produk (satu per baris)'
                        value={form.product_ids_raw}
                        onChange={set('product_ids_raw')}
                        multiline
                        rows={3}
                        placeholder={'Kavling Standar\nKavling Keluarga\nKavling Premium\nKavling VIP Garden'}
                        helperText='Kosong = input teks bebas'
                    />
                    <TextField
                        size='small'
                        fullWidth
                        label='Catatan di bawah form'
                        value={form.form_note}
                        onChange={set('form_note')}
                        placeholder='Dengan mengisi form ini, Anda setuju dihubungi tim kami.'
                    />

                    <Divider />
                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        REDIRECT SETELAH SUBMIT
                    </Typography>
                    <FormControl size='small' fullWidth>
                        <InputLabel>Tipe Redirect</InputLabel>
                        <Select value={form.redirect_type} label='Tipe Redirect' onChange={set('redirect_type')}>
                            {REDIRECT_TYPES.map((t) => (
                                <MenuItem key={t.value} value={t.value}>
                                    <Stack direction='row' spacing={1} alignItems='center'>
                                        {t.icon}
                                        <Box>
                                            <Typography variant='body2'>{t.label}</Typography>
                                            <Typography variant='caption' color='text.secondary'>
                                                {t.desc}
                                            </Typography>
                                        </Box>
                                    </Stack>
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    {form.redirect_type !== 'wa' && (
                        <TextField
                            size='small'
                            fullWidth
                            label={form.redirect_type === 'website' ? 'URL Website' : 'Custom Link'}
                            value={form.redirect_url}
                            onChange={set('redirect_url')}
                            placeholder='https://...'
                        />
                    )}

                    <Divider />
                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        CUSTOM CODE
                    </Typography>
                    <TextField
                        size='small'
                        fullWidth
                        label='Custom JavaScript'
                        value={form.custom_script}
                        onChange={set('custom_script')}
                        multiline
                        rows={4}
                        placeholder={'// Script dijalankan setelah halaman dimuat\nconsole.log("hello");'}
                        inputProps={{ style: { fontFamily: 'monospace', fontSize: 12 } }}
                    />
                    <TextField
                        size='small'
                        fullWidth
                        label='Custom HTML'
                        value={form.custom_html}
                        onChange={set('custom_html')}
                        multiline
                        rows={4}
                        placeholder={'<!-- HTML disisipkan di atas form -->\n<div>...</div>'}
                        inputProps={{ style: { fontFamily: 'monospace', fontSize: 12 } }}
                    />

                    <Divider />
                    <Stack direction='row' alignItems='center' spacing={1}>
                        <Typography variant='body2'>Status:</Typography>
                        <Switch
                            checked={form.status === 'active'}
                            onChange={(e) => setForm((f) => ({ ...f, status: e.target.checked ? 'active' : 'inactive' }))}
                        />
                        <Chip
                            size='small'
                            label={form.status === 'active' ? 'Aktif' : 'Nonaktif'}
                            color={form.status === 'active' ? 'success' : 'default'}
                        />
                    </Stack>

                    <Divider />
                    <Stack direction='row' spacing={1} justifyContent='flex-end'>
                        <Chip label='Batal' onClick={onClose} variant='outlined' clickable />
                        <Chip
                            label={saving ? 'Menyimpan...' : 'Simpan'}
                            color='primary'
                            clickable
                            onClick={save}
                            disabled={saving}
                            icon={saving ? <CircularProgress size={12} color='inherit' /> : undefined}
                        />
                    </Stack>
                </Stack>
            </DialogContent>
        </Dialog>
    )
}
CampaignDialog.propTypes = {
    open: PropTypes.bool,
    campaign: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired
}

// ── Preview mode ──────────────────────────────────────────────────────────────

function CampaignPreview({ campaign, onBack }) {
    return (
        <Box>
            <Stack direction='row' spacing={1} alignItems='center' sx={{ mb: 2 }}>
                <IconButton
                    onClick={onBack}
                    size='small'
                    sx={{ color: 'text.primary', bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
                >
                    <IconArrowLeft size={18} />
                </IconButton>
                <Typography variant='h6'>Preview: {campaign.name}</Typography>
                <Chip size='small' label={getLandingURL(campaign.slug)} variant='outlined' sx={{ fontFamily: 'monospace', fontSize: 11 }} />
            </Stack>
            <Paper variant='outlined' sx={{ borderRadius: 2, overflow: 'hidden' }}>
                <Box sx={{ bgcolor: 'action.hover', px: 2, py: 1 }}>
                    <Stack direction='row' spacing={1} alignItems='center'>
                        <Box sx={{ width: 10, height: 10, borderRadius: '50%', bgcolor: '#ff5f57' }} />
                        <Box sx={{ width: 10, height: 10, borderRadius: '50%', bgcolor: '#ffbd2e' }} />
                        <Box sx={{ width: 10, height: 10, borderRadius: '50%', bgcolor: '#28c840' }} />
                        <Typography variant='caption' color='text.secondary' sx={{ ml: 1, fontFamily: 'monospace' }}>
                            {getLandingURL(campaign.slug)}
                        </Typography>
                    </Stack>
                </Box>
                <Box sx={{ bgcolor: '#f8f9fa', minHeight: 600 }}>
                    <CampaignLanding previewData={campaign} />
                </Box>
            </Paper>
        </Box>
    )
}
CampaignPreview.propTypes = { campaign: PropTypes.object.isRequired, onBack: PropTypes.func.isRequired }

// ── Main component ────────────────────────────────────────────────────────────

export default function CRMCampaigns() {
    const [campaigns, setCampaigns] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)
    const [dialogOpen, setDialogOpen] = useState(false)
    const [editing, setEditing] = useState(null)
    const [previewing, setPreviewing] = useState(null)
    const [snack, setSnack] = useState(null)

    const fetchCampaigns = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const res = await fetch(`${API}/campaigns`)
            setCampaigns(await res.json())
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        fetchCampaigns()
    }, [fetchCampaigns])

    const openAdd = () => {
        setEditing(null)
        setDialogOpen(true)
    }
    const openEdit = (c) => {
        setEditing(c)
        setDialogOpen(true)
    }

    const handleDelete = async (c) => {
        if (!window.confirm(`Hapus campaign "${c.name}"?`)) return
        await fetch(`${API}/campaigns/${c.id}`, { method: 'DELETE' })
        fetchCampaigns()
    }

    const copyURL = (slug) => {
        navigator.clipboard.writeText(getLandingURL(slug))
        setSnack('URL berhasil disalin!')
    }

    if (previewing) {
        return <CampaignPreview campaign={previewing} onBack={() => setPreviewing(null)} />
    }

    return (
        <MainCard
            title='CRM — Campaign Marketing'
            secondary={
                <Stack direction='row' spacing={1}>
                    <Chip size='small' label='+ Buat Campaign' color='primary' clickable onClick={openAdd} icon={<IconPlus size={12} />} />
                    <IconButton size='small' onClick={fetchCampaigns} disabled={loading} sx={{ color: 'text.primary' }}>
                        {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                    </IconButton>
                </Stack>
            }
        >
            {error && (
                <Alert severity='error' sx={{ mb: 2 }}>
                    {error}
                </Alert>
            )}

            <TableContainer component={Paper} variant='outlined'>
                <Table size='small'>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama Campaign</TableCell>
                            <TableCell>URL Landing Page</TableCell>
                            <TableCell>Pixels</TableCell>
                            <TableCell>Redirect</TableCell>
                            <TableCell align='center'>Leads</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell>Dibuat</TableCell>
                            <TableCell sx={{ width: 110 }} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {!loading && campaigns.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={8} align='center' sx={{ py: 4, color: 'text.disabled' }}>
                                    Belum ada campaign. Klik &quot;+ Buat Campaign&quot; untuk memulai.
                                </TableCell>
                            </TableRow>
                        )}
                        {(Array.isArray(campaigns) ? campaigns : []).map((c) => {
                            const rdType = REDIRECT_TYPES.find((t) => t.value === c.redirect_type)
                            return (
                                <TableRow key={c.id} hover>
                                    <TableCell>
                                        <Typography variant='body2' fontWeight={600}>
                                            {c.name}
                                        </Typography>
                                        {c.description && (
                                            <Typography
                                                variant='caption'
                                                color='text.secondary'
                                                sx={{
                                                    display: 'block',
                                                    maxWidth: 180,
                                                    overflow: 'hidden',
                                                    textOverflow: 'ellipsis',
                                                    whiteSpace: 'nowrap'
                                                }}
                                            >
                                                {c.description}
                                            </Typography>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        <Stack direction='row' spacing={0.5} alignItems='center'>
                                            <Typography
                                                variant='caption'
                                                sx={{ fontFamily: 'monospace', color: 'primary.main', fontSize: 11 }}
                                            >
                                                /{c.slug}
                                            </Typography>
                                            <Tooltip title='Copy URL'>
                                                <IconButton
                                                    size='small'
                                                    onClick={() => copyURL(c.slug)}
                                                    sx={{
                                                        color: 'text.primary',
                                                        bgcolor: 'action.hover',
                                                        '&:hover': { bgcolor: 'action.selected' }
                                                    }}
                                                >
                                                    <IconCopy size={11} />
                                                </IconButton>
                                            </Tooltip>
                                        </Stack>
                                    </TableCell>
                                    <TableCell>
                                        <Stack direction='row' spacing={0.5} flexWrap='wrap'>
                                            {(c.pixels || []).map((px, i) => {
                                                const meta = PIXEL_TYPES.find((t) => t.value === px.type)
                                                return (
                                                    <Chip
                                                        key={i}
                                                        size='small'
                                                        variant='outlined'
                                                        label={meta?.label?.split(' ')[0] || px.type}
                                                        icon={meta?.icon}
                                                        sx={{ fontSize: 10 }}
                                                    />
                                                )
                                            })}
                                            {(!c.pixels || c.pixels.length === 0) && (
                                                <Typography variant='caption' color='text.disabled'>
                                                    —
                                                </Typography>
                                            )}
                                        </Stack>
                                    </TableCell>
                                    <TableCell>
                                        {rdType && (
                                            <Stack direction='row' spacing={0.5} alignItems='center'>
                                                {rdType.icon}
                                                <Typography variant='caption'>{rdType.label}</Typography>
                                            </Stack>
                                        )}
                                    </TableCell>
                                    <TableCell align='center'>
                                        <Typography variant='body2' fontWeight={600} color='primary.main'>
                                            {c.leads_count ?? 0}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Chip
                                            size='small'
                                            label={c.status === 'active' ? 'Aktif' : 'Nonaktif'}
                                            color={c.status === 'active' ? 'success' : 'default'}
                                        />
                                    </TableCell>
                                    <TableCell>
                                        <Typography variant='caption' color='text.secondary'>
                                            {new Date(c.created_at).toLocaleDateString('id-ID', {
                                                day: 'numeric',
                                                month: 'short',
                                                year: 'numeric'
                                            })}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Stack direction='row' spacing={0.5}>
                                            <Tooltip title='Preview'>
                                                <IconButton
                                                    size='small'
                                                    onClick={() => setPreviewing(c)}
                                                    sx={{
                                                        color: 'primary.main',
                                                        bgcolor: 'action.hover',
                                                        '&:hover': { bgcolor: 'action.selected' }
                                                    }}
                                                >
                                                    <IconEye size={14} />
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title='Edit'>
                                                <IconButton
                                                    size='small'
                                                    onClick={() => openEdit(c)}
                                                    sx={{
                                                        color: 'text.primary',
                                                        bgcolor: 'action.hover',
                                                        '&:hover': { bgcolor: 'action.selected' }
                                                    }}
                                                >
                                                    <IconEdit size={14} />
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title='Hapus'>
                                                <IconButton
                                                    size='small'
                                                    color='error'
                                                    onClick={() => handleDelete(c)}
                                                    sx={{ bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
                                                >
                                                    <IconTrash size={14} />
                                                </IconButton>
                                            </Tooltip>
                                        </Stack>
                                    </TableCell>
                                </TableRow>
                            )
                        })}
                    </TableBody>
                </Table>
            </TableContainer>

            <Box sx={{ mt: 1 }}>
                <Typography variant='caption' color='text.disabled'>
                    URL landing page: <code>{CAMPAIGN_DOMAIN}/[slug]</code> — dapat dibagikan ke media sosial tanpa login.
                </Typography>
            </Box>

            <CampaignDialog open={dialogOpen} campaign={editing} onClose={() => setDialogOpen(false)} onSaved={fetchCampaigns} />

            <Snackbar
                open={Boolean(snack)}
                autoHideDuration={2500}
                onClose={() => setSnack(null)}
                message={snack}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            />
        </MainCard>
    )
}
