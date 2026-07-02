import PropTypes from 'prop-types'
import { useEffect, useState, useCallback } from 'react'
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
    TableRow,
    TextField,
    Tooltip,
    Typography
} from '@mui/material'
import { IconRefresh, IconPlus, IconEdit, IconTrash, IconPhone, IconBrandTelegram, IconMail, IconTrendingUp } from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/crm'

const EMPTY_FORM = {
    name: '',
    phone: '',
    telegram_id: '',
    telegram_chat_id: '',
    email: '',
    area: '',
    commission_type: 'percentage',
    commission_rate: '',
    target_monthly: 10,
    status: 'active',
    notes: ''
}

function fmtCurrency(n) {
    if (!n) return '—'
    return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(n)
}

function StatChip({ label, value, color }) {
    return (
        <Stack alignItems='center' sx={{ minWidth: 60 }}>
            <Typography variant='h6' sx={{ color: color || 'text.primary', fontWeight: 700, lineHeight: 1 }}>
                {value}
            </Typography>
            <Typography variant='caption' color='text.disabled' sx={{ fontSize: 10 }}>
                {label}
            </Typography>
        </Stack>
    )
}
StatChip.propTypes = { label: PropTypes.string, value: PropTypes.any, color: PropTypes.string }

function SalesmanDialog({ open, salesman, onClose, onSaved }) {
    const [form, setForm] = useState(EMPTY_FORM)
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState(null)

    useEffect(() => {
        setForm(salesman ? { ...EMPTY_FORM, ...salesman } : EMPTY_FORM)
        setError(null)
    }, [salesman, open])

    const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

    const save = async () => {
        if (!form.name.trim()) {
            setError('Nama wajib diisi')
            return
        }
        setSaving(true)
        setError(null)
        try {
            const url = salesman ? `${API}/salesmen/${salesman.id}` : `${API}/salesmen`
            const res = await fetch(url, {
                method: salesman ? 'PUT' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    ...form,
                    commission_rate: parseFloat(form.commission_rate) || 0,
                    target_monthly: parseInt(form.target_monthly) || 10
                })
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
            <DialogTitle>{salesman ? 'Edit Salesman' : 'Tambah Salesman'}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ mt: 1 }}>
                    {error && <Alert severity='error'>{error}</Alert>}

                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        DATA DIRI
                    </Typography>

                    <Grid container spacing={1.5}>
                        <Grid item xs={12}>
                            <TextField size='small' fullWidth label='Nama Lengkap *' value={form.name} onChange={set('name')} />
                        </Grid>
                        <Grid item xs={6}>
                            <TextField
                                size='small'
                                fullWidth
                                label='No. WhatsApp'
                                value={form.phone}
                                onChange={set('phone')}
                                placeholder='628123456789'
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
                            <TextField
                                size='small'
                                fullWidth
                                label='Email'
                                value={form.email}
                                onChange={set('email')}
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position='start'>
                                            <IconMail size={14} />
                                        </InputAdornment>
                                    )
                                }}
                            />
                        </Grid>
                        <Grid item xs={6}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Telegram @username'
                                value={form.telegram_id}
                                onChange={set('telegram_id')}
                                placeholder='@username'
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position='start'>
                                            <IconBrandTelegram size={14} />
                                        </InputAdornment>
                                    )
                                }}
                            />
                        </Grid>
                        <Grid item xs={6}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Telegram Chat ID'
                                value={form.telegram_chat_id}
                                onChange={set('telegram_chat_id')}
                                placeholder='123456789 (untuk notif langsung)'
                            />
                        </Grid>
                        <Grid item xs={12}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Area Coverage'
                                value={form.area}
                                onChange={set('area')}
                                placeholder='Karawang, Jakarta Timur, Bekasi'
                            />
                        </Grid>
                    </Grid>

                    <Divider />
                    <Typography variant='caption' color='text.secondary' sx={{ fontWeight: 600 }}>
                        TARGET & KOMISI
                    </Typography>

                    <Grid container spacing={1.5}>
                        <Grid item xs={6}>
                            <TextField
                                size='small'
                                fullWidth
                                label='Target Leads/Bulan'
                                type='number'
                                value={form.target_monthly}
                                onChange={set('target_monthly')}
                            />
                        </Grid>
                        <Grid item xs={6}>
                            <FormControl size='small' fullWidth>
                                <InputLabel>Tipe Komisi</InputLabel>
                                <Select value={form.commission_type} label='Tipe Komisi' onChange={set('commission_type')}>
                                    <MenuItem value='percentage'>Persentase (%)</MenuItem>
                                    <MenuItem value='flat'>Flat per Deal (Rp)</MenuItem>
                                </Select>
                            </FormControl>
                        </Grid>
                        <Grid item xs={6}>
                            <TextField
                                size='small'
                                fullWidth
                                label={form.commission_type === 'percentage' ? 'Komisi (%)' : 'Komisi per Deal (Rp)'}
                                type='number'
                                value={form.commission_rate}
                                onChange={set('commission_rate')}
                                InputProps={{
                                    startAdornment: (
                                        <InputAdornment position='start'>
                                            {form.commission_type === 'percentage' ? '%' : 'Rp'}
                                        </InputAdornment>
                                    )
                                }}
                            />
                        </Grid>
                        <Grid item xs={6}>
                            <FormControl size='small' fullWidth>
                                <InputLabel>Status</InputLabel>
                                <Select value={form.status} label='Status' onChange={set('status')}>
                                    <MenuItem value='active'>Aktif</MenuItem>
                                    <MenuItem value='inactive'>Nonaktif</MenuItem>
                                </Select>
                            </FormControl>
                        </Grid>
                    </Grid>

                    <TextField size='small' fullWidth label='Catatan' multiline rows={2} value={form.notes} onChange={set('notes')} />

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

SalesmanDialog.propTypes = {
    open: PropTypes.bool,
    salesman: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired
}

export default function CRMSalesmen() {
    const [salesmen, setSalesmen] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)
    const [dialogOpen, setDialogOpen] = useState(false)
    const [editing, setEditing] = useState(null)

    const fetchSalesmen = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const res = await fetch(`${API}/salesmen`)
            setSalesmen(await res.json())
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        fetchSalesmen()
    }, [fetchSalesmen])

    const openAdd = () => {
        setEditing(null)
        setDialogOpen(true)
    }
    const openEdit = (s) => {
        setEditing(s)
        setDialogOpen(true)
    }

    const handleDelete = async (s) => {
        if (!window.confirm(`Hapus salesman "${s.name}"? Leads yang sudah di-assign tidak akan berubah.`)) return
        await fetch(`${API}/salesmen/${s.id}`, { method: 'DELETE' })
        fetchSalesmen()
    }

    const totalActive = Array.isArray(salesmen) ? salesmen.filter((s) => s.status === 'active').length : 0

    return (
        <MainCard
            title='CRM — Tim Salesman'
            secondary={
                <Stack direction='row' spacing={1}>
                    <Chip
                        size='small'
                        label='+ Tambah Salesman'
                        color='primary'
                        clickable
                        onClick={openAdd}
                        icon={<IconPlus size={12} />}
                    />
                    <IconButton size='small' onClick={fetchSalesmen} disabled={loading}>
                        {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                    </IconButton>
                </Stack>
            }
        >
            <Stack direction='row' spacing={2} sx={{ mb: 2 }}>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                    <Typography variant='h5' color='primary.main' fontWeight={700}>
                        {totalActive}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Aktif
                    </Typography>
                </Paper>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                    <Typography variant='h5' fontWeight={700}>
                        {Array.isArray(salesmen) ? salesmen.length : 0}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Total
                    </Typography>
                </Paper>
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
                            <TableCell>Kontak</TableCell>
                            <TableCell>Area</TableCell>
                            <TableCell align='center'>Target/bln</TableCell>
                            <TableCell align='center'>Leads bln ini</TableCell>
                            <TableCell align='center'>Total / Won</TableCell>
                            <TableCell align='center'>Konversi</TableCell>
                            <TableCell>Komisi</TableCell>
                            <TableCell>Est. Komisi</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell sx={{ width: 72 }} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {!loading && salesmen.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={11} align='center' sx={{ py: 4, color: 'text.disabled' }}>
                                    Belum ada salesman. Klik &quot;+ Tambah Salesman&quot; untuk memulai.
                                </TableCell>
                            </TableRow>
                        )}
                        {(Array.isArray(salesmen) ? salesmen : []).map((s) => (
                            <TableRow key={s.id} hover>
                                <TableCell>
                                    <Typography variant='body2' fontWeight={600}>
                                        {s.name}
                                    </Typography>
                                    {s.notes && (
                                        <Typography variant='caption' color='text.disabled'>
                                            {s.notes}
                                        </Typography>
                                    )}
                                </TableCell>
                                <TableCell>
                                    <Stack spacing={0.3}>
                                        {s.phone && (
                                            <Stack direction='row' spacing={0.5} alignItems='center'>
                                                <IconPhone size={12} />
                                                <Typography variant='caption' sx={{ fontFamily: 'monospace' }}>
                                                    {s.phone}
                                                </Typography>
                                            </Stack>
                                        )}
                                        {s.telegram_id && (
                                            <Stack direction='row' spacing={0.5} alignItems='center'>
                                                <IconBrandTelegram size={12} color='#2AABEE' />
                                                <Typography variant='caption'>{s.telegram_id}</Typography>
                                            </Stack>
                                        )}
                                        {s.email && (
                                            <Stack direction='row' spacing={0.5} alignItems='center'>
                                                <IconMail size={12} />
                                                <Typography variant='caption'>{s.email}</Typography>
                                            </Stack>
                                        )}
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {s.area || '—'}
                                    </Typography>
                                </TableCell>
                                <TableCell align='center'>
                                    <Typography variant='body2'>{s.target_monthly}</Typography>
                                </TableCell>
                                <TableCell align='center'>
                                    <Box>
                                        <Typography
                                            variant='body2'
                                            fontWeight={600}
                                            color={s.leads_this_month >= s.target_monthly ? 'success.main' : 'text.primary'}
                                        >
                                            {s.leads_this_month ?? 0}
                                        </Typography>
                                        {s.target_monthly > 0 && (
                                            <Typography variant='caption' color='text.disabled'>
                                                / {s.target_monthly}
                                            </Typography>
                                        )}
                                    </Box>
                                </TableCell>
                                <TableCell align='center'>
                                    <Typography variant='caption'>
                                        {s.leads_total ?? 0} / {s.leads_won ?? 0}
                                    </Typography>
                                </TableCell>
                                <TableCell align='center'>
                                    <Stack direction='row' spacing={0.5} alignItems='center' justifyContent='center'>
                                        <IconTrendingUp size={12} color={s.conversion_rate > 20 ? '#22c55e' : '#f97316'} />
                                        <Typography
                                            variant='caption'
                                            sx={{ color: s.conversion_rate > 20 ? 'success.main' : 'warning.main' }}
                                        >
                                            {s.conversion_rate ? s.conversion_rate.toFixed(1) : '0'}%
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption'>
                                        {s.commission_type === 'percentage'
                                            ? `${s.commission_rate}%`
                                            : fmtCurrency(s.commission_rate) + '/deal'}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='success.main' fontWeight={600}>
                                        {fmtCurrency(s.est_commission)}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        size='small'
                                        label={s.status === 'active' ? 'Aktif' : 'Nonaktif'}
                                        color={s.status === 'active' ? 'success' : 'default'}
                                    />
                                </TableCell>
                                <TableCell>
                                    <Stack direction='row' spacing={0.5}>
                                        <Tooltip title='Edit'>
                                            <IconButton
                                                size='small'
                                                onClick={() => openEdit(s)}
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
                                                onClick={() => handleDelete(s)}
                                                sx={{ bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
                                            >
                                                <IconTrash size={14} />
                                            </IconButton>
                                        </Tooltip>
                                    </Stack>
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </TableContainer>

            <Box sx={{ mt: 1 }}>
                <Typography variant='caption' color='text.disabled'>
                    Leads baru akan di-assign otomatis ke salesman aktif dengan leads paling sedikit bulan ini. Est. Komisi dihitung dari
                    leads Won × asumsi avg kavling Rp 50 juta.
                </Typography>
            </Box>

            <SalesmanDialog open={dialogOpen} salesman={editing} onClose={() => setDialogOpen(false)} onSaved={fetchSalesmen} />
        </MainCard>
    )
}
