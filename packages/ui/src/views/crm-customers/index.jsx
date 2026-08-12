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
    InputAdornment,
    InputLabel,
    MenuItem,
    Paper,
    Select,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TablePagination,
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
    IconPhone,
    IconSearch,
    IconStar,
    IconBan,
    IconFileSpreadsheet,
    IconDownload,
    IconUpload
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/crm'
const AUTH_HEADER = { 'x-request-from': 'internal' }
const JSON_HEADERS = { ...AUTH_HEADER, 'Content-Type': 'application/json' }

const EMPTY_FORM = {
    name: '',
    phone: '',
    wilayah: '',
    // "normal" (adj forced to 0, no markup at all) must be a deliberate
    // choice — the default for a newly added customer is "unregistered",
    // which gets the standard markup like any not-yet-classified number.
    tier: 'unregistered',
    adj: '',
    notes: ''
}

const TIER_META = {
    vip: { label: 'VIP', color: 'success', icon: <IconStar size={12} /> },
    blacklist: { label: 'Blacklist', color: 'error', icon: <IconBan size={12} /> },
    normal: { label: 'Normal', color: 'default', icon: undefined },
    unregistered: { label: 'Belum Terdaftar', color: 'default', icon: undefined }
}

// Reads a response as JSON, but fails with a clear, readable message
// (including HTTP status + a snippet of the actual body) instead of an
// opaque "Unexpected token '<'" when the server/proxy returns something
// that isn't JSON at all (e.g. an nginx error page during a deploy).
async function parseJsonResponse(res) {
    const text = await res.text()
    try {
        return JSON.parse(text)
    } catch {
        throw new Error(`Server balas non-JSON (HTTP ${res.status}): ${text.slice(0, 150) || '(kosong)'}`)
    }
}

function downloadFile(url) {
    const a = document.createElement('a')
    a.href = url
    a.download = ''
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
}

function CustomerDialog({ open, customer, onClose, onSaved }) {
    const [form, setForm] = useState(EMPTY_FORM)
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState(null)

    useEffect(() => {
        setForm(customer ? { ...EMPTY_FORM, ...customer, phone: (customer.phone || []).join(', ') } : EMPTY_FORM)
        setError(null)
    }, [customer, open])

    const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

    // Adj sign follows the tier: VIP = discount (negative), Blacklist = markup (positive).
    // The form just takes a plain percentage; the sign is applied on save.
    const save = async () => {
        if (!form.name.trim()) {
            setError('Nama wajib diisi')
            return
        }
        setSaving(true)
        setError(null)
        try {
            const pct = Math.abs(parseFloat(form.adj) || 0)
            const adj = form.tier === 'blacklist' ? pct : form.tier === 'vip' ? -pct : 0
            const url = customer ? `${API}/customers/${customer.id}` : `${API}/customers`
            const res = await fetch(url, {
                method: customer ? 'PUT' : 'POST',
                headers: JSON_HEADERS,
                body: JSON.stringify({ ...form, adj })
            })
            if (!res.ok) throw new Error('Gagal menyimpan')
            onSaved()
            onClose()
        } catch (e) {
            setError(e.message)
        } finally {
            setSaving(false)
        }
    }

    return (
        <Dialog open={open} onClose={onClose} maxWidth='sm' fullWidth>
            <DialogTitle>{customer ? 'Edit Customer' : 'Tambah Customer'}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ mt: 1 }}>
                    {error && <Alert severity='error'>{error}</Alert>}

                    <Grid container spacing={1.5}>
                        <Grid item xs={12}>
                            <TextField size='small' fullWidth label='Nama *' value={form.name} onChange={set('name')} />
                        </Grid>
                        <Grid item xs={12}>
                            <TextField
                                size='small'
                                fullWidth
                                label='No. WhatsApp'
                                value={form.phone}
                                onChange={set('phone')}
                                placeholder='628123456789, 08123456789 (pisahkan koma kalau lebih dari 1 nomor)'
                                helperText='Boleh lebih dari satu nomor untuk customer yang sama, pisahkan dengan koma'
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position='start'>
                                            <IconPhone size={14} />
                                        </InputAdornment>
                                    )
                                }}
                            />
                        </Grid>
                        <Grid item xs={6}>
                            <TextField size='small' fullWidth label='Wilayah' value={form.wilayah} onChange={set('wilayah')} />
                        </Grid>
                        <Grid item xs={6}>
                            <FormControl size='small' fullWidth>
                                <InputLabel>Flag</InputLabel>
                                <Select value={form.tier} label='Flag' onChange={set('tier')}>
                                    <MenuItem value='unregistered'>Belum Terdaftar (markup default)</MenuItem>
                                    <MenuItem value='normal'>Normal (tanpa penyesuaian)</MenuItem>
                                    <MenuItem value='vip'>VIP (diskon)</MenuItem>
                                    <MenuItem value='blacklist'>Blacklist (markup)</MenuItem>
                                </Select>
                            </FormControl>
                        </Grid>
                        {form.tier !== 'normal' && form.tier !== 'unregistered' && (
                            <Grid item xs={6}>
                                <TextField
                                    size='small'
                                    fullWidth
                                    type='number'
                                    label={form.tier === 'vip' ? 'Diskon (%)' : 'Markup (%)'}
                                    value={form.adj}
                                    onChange={set('adj')}
                                    InputProps={{ startAdornment: <InputAdornment position='start'>%</InputAdornment> }}
                                />
                            </Grid>
                        )}
                        {form.tier === 'unregistered' && (
                            <Grid item xs={12}>
                                <Typography variant='caption' color='text.secondary'>
                                    Kena markup default (bisa berubah kalau setting-nya diubah) — bukan angka tetap.
                                </Typography>
                            </Grid>
                        )}
                        <Grid item xs={12}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Catatan'
                                multiline
                                rows={2}
                                value={form.notes}
                                onChange={set('notes')}
                            />
                        </Grid>
                    </Grid>

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

CustomerDialog.propTypes = {
    open: PropTypes.bool,
    customer: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired
}

function ImportResultDialog({ result, onClose }) {
    return (
        <Dialog open={!!result} onClose={onClose} maxWidth='xs' fullWidth>
            <DialogTitle>Hasil Import</DialogTitle>
            <DialogContent>
                {result?.error ? (
                    <Alert severity='error'>{result.error}</Alert>
                ) : (
                    <Stack spacing={1.5} sx={{ mt: 1 }}>
                        <Stack direction='row' spacing={2}>
                            <Chip color='success' label={`${result?.created ?? 0} baru dibuat`} />
                            <Chip color='info' label={`${result?.updated ?? 0} di-update`} />
                        </Stack>
                        {result?.errors?.length > 0 && (
                            <Alert severity='warning'>
                                {result.errors.length} baris gagal:
                                <Box component='ul' sx={{ m: 0, pl: 2 }}>
                                    {result.errors.map((e, i) => (
                                        <li key={i}>
                                            Baris {e.row} ({e.name}): {e.error}
                                        </li>
                                    ))}
                                </Box>
                            </Alert>
                        )}
                    </Stack>
                )}
                <Stack direction='row' justifyContent='flex-end' sx={{ mt: 2 }}>
                    <Chip label='Tutup' onClick={onClose} variant='outlined' clickable />
                </Stack>
            </DialogContent>
        </Dialog>
    )
}
ImportResultDialog.propTypes = { result: PropTypes.object, onClose: PropTypes.func.isRequired }

export default function CRMCustomers() {
    const [customers, setCustomers] = useState([])
    const [total, setTotal] = useState(0)
    const [tierCounts, setTierCounts] = useState({ vip: 0, blacklist: 0 })
    const [q, setQ] = useState('')
    const [page, setPage] = useState(0) // MUI TablePagination is 0-indexed
    const [rowsPerPage, setRowsPerPage] = useState(25)
    const [loading, setLoading] = useState(false)
    const [importing, setImporting] = useState(false)
    const [error, setError] = useState(null)
    const [dialogOpen, setDialogOpen] = useState(false)
    const [editing, setEditing] = useState(null)
    const [importResult, setImportResult] = useState(null)
    const [importProgress, setImportProgress] = useState(null)
    const fileInputRef = useRef(null)

    const fetchCustomers = useCallback(async (query, pageIdx, limit) => {
        setLoading(true)
        setError(null)
        try {
            const params = new URLSearchParams({ page: pageIdx + 1, limit })
            if (query) params.set('q', query)
            const res = await fetch(`${API}/customers?${params}`, { headers: AUTH_HEADER })
            const data = await res.json()
            setCustomers(data.items || [])
            setTotal(data.total || 0)
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [])

    const fetchTierCounts = useCallback(async () => {
        const [vip, blacklist] = await Promise.all([
            fetch(`${API}/customers?tier=vip&limit=1`, { headers: AUTH_HEADER }).then((r) => r.json()),
            fetch(`${API}/customers?tier=blacklist&limit=1`, { headers: AUTH_HEADER }).then((r) => r.json())
        ])
        setTierCounts({ vip: vip.total || 0, blacklist: blacklist.total || 0 })
    }, [])

    const refresh = useCallback(() => {
        fetchCustomers(q, page, rowsPerPage)
        fetchTierCounts()
    }, [fetchCustomers, fetchTierCounts, q, page, rowsPerPage])

    // Refetch on pagination changes only — search is explicit (Enter/icon), not per-keystroke.
    useEffect(() => {
        fetchCustomers(q, page, rowsPerPage)
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, rowsPerPage])

    useEffect(() => {
        fetchTierCounts()
    }, [fetchTierCounts])

    const handleSearch = () => {
        if (page === 0) fetchCustomers(q, 0, rowsPerPage)
        else setPage(0) // triggers the pagination effect above
    }

    const openAdd = () => {
        setEditing(null)
        setDialogOpen(true)
    }
    const openEdit = (c) => {
        setEditing(c)
        setDialogOpen(true)
    }

    const handleDelete = async (c) => {
        if (!window.confirm(`Hapus customer "${c.name}"?`)) return
        await fetch(`${API}/customers/${c.id}`, { method: 'DELETE', headers: AUTH_HEADER })
        refresh()
    }

    const pollImportJob = async (jobId) => {
        for (;;) {
            await new Promise((resolve) => setTimeout(resolve, 1000))
            const res = await fetch(`${API}/customers/import/${jobId}`, { headers: AUTH_HEADER })
            const data = await parseJsonResponse(res)
            if (!res.ok) throw new Error(data.error || 'Gagal cek status import')
            setImportProgress(data) // live processed/total while it runs
            if (data.status === 'done') return data
        }
    }

    const handleImportFile = async (e) => {
        const file = e.target.files?.[0]
        e.target.value = '' // allow re-selecting the same file next time
        if (!file) return
        setImporting(true)
        setImportProgress(null)
        try {
            const form = new FormData()
            form.append('file', file)
            const res = await fetch(`${API}/customers/import`, { method: 'POST', headers: AUTH_HEADER, body: form })
            const data = await parseJsonResponse(res)
            if (!res.ok) throw new Error(data.error || 'Gagal import')
            const result = await pollImportJob(data.job_id)
            setImportResult(result)
            refresh()
        } catch (e) {
            setImportResult({ error: e.message })
        } finally {
            setImporting(false)
            setImportProgress(null)
        }
    }

    const list = Array.isArray(customers) ? customers : []

    return (
        <MainCard
            title='CRM — Customers (VIP / Blacklist)'
            secondary={
                <Stack direction='row' spacing={1}>
                    <Tooltip title='Download Template Excel'>
                        <IconButton size='small' onClick={() => downloadFile(`${API}/customers/template`)} sx={{ color: 'text.secondary' }}>
                            <IconFileSpreadsheet size={18} />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title='Export ke Excel'>
                        <IconButton size='small' onClick={() => downloadFile(`${API}/customers/export`)} sx={{ color: 'text.secondary' }}>
                            <IconDownload size={18} />
                        </IconButton>
                    </Tooltip>
                    <Tooltip
                        title={
                            importing && importProgress
                                ? `Memproses ${importProgress.processed}/${importProgress.total}...`
                                : 'Import dari Excel (sync — data lama ter-update, bukan dobel, maks 20.000 baris)'
                        }
                    >
                        <span>
                            <IconButton
                                size='small'
                                onClick={() => fileInputRef.current?.click()}
                                disabled={importing}
                                sx={{ color: 'text.secondary' }}
                            >
                                {importing ? <CircularProgress size={16} /> : <IconUpload size={18} />}
                            </IconButton>
                        </span>
                    </Tooltip>
                    {importing && importProgress && (
                        <Typography variant='caption' color='text.secondary' sx={{ alignSelf: 'center' }}>
                            {importProgress.processed}/{importProgress.total}
                        </Typography>
                    )}
                    <input ref={fileInputRef} type='file' accept='.xlsx' hidden onChange={handleImportFile} />
                    <Chip size='small' label='Tambah Customer' color='primary' clickable onClick={openAdd} icon={<IconPlus size={14} />} />
                    <IconButton size='small' onClick={refresh} disabled={loading} sx={{ color: 'text.secondary' }}>
                        {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                    </IconButton>
                </Stack>
            }
        >
            <Stack direction='row' spacing={2} sx={{ mb: 2 }} alignItems='center'>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                    <Typography variant='h5' color='success.main' fontWeight={700}>
                        {tierCounts.vip}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        VIP
                    </Typography>
                </Paper>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                    <Typography variant='h5' color='error.main' fontWeight={700}>
                        {tierCounts.blacklist}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Blacklist
                    </Typography>
                </Paper>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                    <Typography variant='h5' fontWeight={700}>
                        {total}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Total
                    </Typography>
                </Paper>
                <TextField
                    size='small'
                    placeholder='Cari nama, wilayah, atau no. HP...'
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                    sx={{ flex: 1, maxWidth: 320 }}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position='start'>
                                <IconButton size='small' onClick={handleSearch} sx={{ ml: -1 }}>
                                    <IconSearch size={16} />
                                </IconButton>
                            </InputAdornment>
                        )
                    }}
                />
            </Stack>

            {error && (
                <Alert severity='error' sx={{ mb: 2 }}>
                    {error}
                </Alert>
            )}

            <TableContainer component={Paper} variant='outlined'>
                <Table size='small'>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama</TableCell>
                            <TableCell>No. WhatsApp</TableCell>
                            <TableCell>Wilayah</TableCell>
                            <TableCell>Flag</TableCell>
                            <TableCell align='right'>Penyesuaian</TableCell>
                            <TableCell>Catatan</TableCell>
                            <TableCell sx={{ width: 72 }} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {!loading && list.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={7} align='center' sx={{ py: 4, color: 'text.secondary' }}>
                                    Belum ada customer. Klik &quot;+ Tambah Customer&quot; atau import dari Excel untuk memulai.
                                </TableCell>
                            </TableRow>
                        )}
                        {list.map((c) => {
                            const meta = TIER_META[c.tier] || TIER_META.unregistered
                            return (
                                <TableRow key={c.id} hover>
                                    <TableCell>
                                        <Typography variant='body2' fontWeight={600}>
                                            {c.name}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Stack spacing={0.3}>
                                            {(c.phone || []).map((p) => (
                                                <Stack key={p} direction='row' spacing={0.5} alignItems='center'>
                                                    <IconPhone size={12} />
                                                    <Typography variant='caption' sx={{ fontFamily: 'monospace' }}>
                                                        {p}
                                                    </Typography>
                                                </Stack>
                                            ))}
                                        </Stack>
                                    </TableCell>
                                    <TableCell>
                                        <Typography variant='caption' color='text.secondary'>
                                            {c.wilayah || '—'}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Chip size='small' label={meta.label} color={meta.color} icon={meta.icon} />
                                    </TableCell>
                                    <TableCell align='right'>
                                        <Typography
                                            variant='caption'
                                            fontWeight={600}
                                            color={c.adj < 0 ? 'success.main' : c.adj > 0 ? 'error.main' : 'text.secondary'}
                                        >
                                            {c.adj > 0 ? '+' : ''}
                                            {c.adj}%
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Typography variant='caption' color='text.secondary'>
                                            {c.notes || '—'}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Stack direction='row' spacing={0.5}>
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
                <TablePagination
                    component='div'
                    count={total}
                    page={page}
                    onPageChange={(_, newPage) => setPage(newPage)}
                    rowsPerPage={rowsPerPage}
                    onRowsPerPageChange={(e) => {
                        setRowsPerPage(parseInt(e.target.value, 10))
                        setPage(0)
                    }}
                    rowsPerPageOptions={[10, 25, 50, 100]}
                    labelRowsPerPage='Baris/halaman'
                    labelDisplayedRows={({ from, to, count }) => `${from}-${to} dari ${count}`}
                />
            </TableContainer>

            <Box sx={{ mt: 1 }}>
                <Typography variant='caption' color='text.secondary'>
                    Customer VIP dapat diskon (adj negatif), Blacklist kena markup (adj positif), Normal tidak ada penyesuaian. Dipakai
                    otomatis oleh bot penjualan (Bona/Bobi) untuk menyesuaikan harga saat customer chat. Import Excel bersifat sync:
                    customer dengan nomor HP yang sudah ada akan di-update, bukan dobel.
                </Typography>
            </Box>

            <CustomerDialog open={dialogOpen} customer={editing} onClose={() => setDialogOpen(false)} onSaved={refresh} />
            <ImportResultDialog result={importResult} onClose={() => setImportResult(null)} />
        </MainCard>
    )
}
