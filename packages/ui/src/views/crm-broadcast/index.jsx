import { useCallback, useEffect, useState } from 'react'
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    IconButton,
    LinearProgress,
    Paper,
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
import { IconEdit, IconEye, IconPlayerPlay, IconPlus, IconRefresh, IconSearch, IconTrash } from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'
import broadcastApi from '@/api/crmbroadcast'
import CampaignDialog from './CampaignDialog'
import CampaignDetail from './CampaignDetail'

const STATUS_COLOR = {
    draft: 'default',
    building: 'info',
    ready: 'info',
    scheduled: 'info',
    running: 'primary',
    paused: 'warning',
    done: 'success',
    cancelled: 'default'
}

const CRMBroadcast = () => {
    const [items, setItems] = useState([])
    const [total, setTotal] = useState(0)
    const [page, setPage] = useState(0)
    const [rowsPerPage, setRowsPerPage] = useState(25)
    const [q, setQ] = useState('')
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState('')
    const [dialogOpen, setDialogOpen] = useState(false)
    const [editing, setEditing] = useState(null)
    const [detailId, setDetailId] = useState(null)

    const fetchBroadcasts = useCallback(
        async (search = q, pageIdx = page, limit = rowsPerPage) => {
            setLoading(true)
            setError('')
            try {
                const { data } = await broadcastApi.getBroadcasts({
                    q: search || undefined,
                    page: pageIdx + 1,
                    limit
                })
                setItems(data.items || [])
                setTotal(data.total || 0)
            } catch (e) {
                setError(e.response?.data?.error || e.message)
            } finally {
                setLoading(false)
            }
        },
        [q, page, rowsPerPage]
    )

    useEffect(() => {
        fetchBroadcasts(q, page, rowsPerPage)
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [page, rowsPerPage])

    const refresh = () => fetchBroadcasts(q, page, rowsPerPage)

    const handleSearch = () => {
        if (page === 0) fetchBroadcasts(q, 0, rowsPerPage)
        else setPage(0)
    }

    const openCreate = () => {
        setEditing(null)
        setDialogOpen(true)
    }

    const openEdit = async (row) => {
        setError('')
        try {
            // Fetch fresh rather than reusing the list row: the list summary
            // never carried the sender label / media info the form needs.
            const { data } = await broadcastApi.getBroadcast(row.id)
            setEditing(data)
            setDialogOpen(true)
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    const buildAndStart = async (row) => {
        setError('')
        try {
            // A campaign must have a materialized audience before it can run.
            // Only for draft/ready: this REPLACES the recipient list, which
            // would wipe a paused campaign's sent/failed history and risk
            // re-sending to numbers it already reached. Paused campaigns must
            // go through resumeBroadcast instead — see resumePaused().
            const { data } = await broadcastApi.buildAudience(row.id)
            const jobId = data.job_id
            for (;;) {
                await new Promise((r) => setTimeout(r, 800))
                const { data: job } = await broadcastApi.getBuildStatus(row.id, jobId)
                if (job.status === 'done') {
                    if (job.errors?.length) throw new Error(job.errors[0].error || 'gagal bangun audiens')
                    break
                }
            }
            await broadcastApi.startBroadcast(row.id)
            refresh()
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    const resumePaused = async (row) => {
        setError('')
        try {
            await broadcastApi.resumeBroadcast(row.id)
            refresh()
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    const remove = async (row) => {
        if (!window.confirm(`Hapus broadcast "${row.name}"?`)) return
        try {
            await broadcastApi.deleteBroadcast(row.id)
            refresh()
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    const statCount = (status) => items.filter((i) => i.status === status).length
    const uncertainTotal = items.reduce((a, i) => a + (i.counts?.uncertain || 0), 0)

    return (
        <MainCard
            title='CRM — Broadcast WhatsApp'
            secondary={
                <Stack direction='row' spacing={1} alignItems='center'>
                    <Chip
                        size='small'
                        label='Buat Broadcast'
                        color='primary'
                        clickable
                        onClick={openCreate}
                        icon={<IconPlus size={14} />}
                    />
                    <IconButton size='small' onClick={refresh} disabled={loading} sx={{ color: 'text.secondary' }}>
                        {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                    </IconButton>
                </Stack>
            }
        >
            <Stack direction='row' spacing={2} sx={{ mb: 2 }} alignItems='center'>
                {[
                    ['Draft', statCount('draft') + statCount('ready'), undefined],
                    ['Terjadwal', statCount('scheduled'), 'info.main'],
                    ['Berjalan', statCount('running'), 'primary.main'],
                    ['Selesai', statCount('done'), 'success.main']
                ].map(([label, value, color]) => (
                    <Paper key={label} variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 90 }}>
                        <Typography variant='h5' fontWeight={700} color={color}>
                            {value}
                        </Typography>
                        <Typography variant='caption' color='text.secondary'>
                            {label}
                        </Typography>
                    </Paper>
                ))}
                {uncertainTotal > 0 && (
                    <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 110, borderColor: 'warning.main' }}>
                        <Typography variant='h5' fontWeight={700} color='warning.main'>
                            {uncertainTotal}
                        </Typography>
                        <Typography variant='caption' color='text.secondary'>
                            Perlu Ditinjau
                        </Typography>
                    </Paper>
                )}
                <TextField
                    size='small'
                    placeholder='Cari nama campaign...'
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') handleSearch()
                    }}
                    sx={{ flex: 1, maxWidth: 320 }}
                    InputProps={{
                        endAdornment: (
                            <IconButton size='small' onClick={handleSearch} sx={{ color: 'text.secondary' }}>
                                <IconSearch size={16} />
                            </IconButton>
                        )
                    }}
                />
            </Stack>

            {error && (
                <Alert severity='error' sx={{ mb: 2 }} onClose={() => setError('')}>
                    {error}
                </Alert>
            )}

            <TableContainer component={Paper} variant='outlined'>
                <Table size='small'>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell>Progress</TableCell>
                            <TableCell>Jadwal</TableCell>
                            <TableCell align='right'>Aksi</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {items.length === 0 && (
                            <TableRow>
                                <TableCell colSpan={5} align='center' sx={{ py: 4, color: 'text.secondary' }}>
                                    Belum ada broadcast. Klik &quot;Buat Broadcast&quot; untuk mulai.
                                </TableCell>
                            </TableRow>
                        )}
                        {items.map((row) => {
                            const c = row.counts || {}
                            const sent = c.sent || 0
                            const planned = sent + (c.failed || 0) + (c.uncertain || 0) + (c.pending || 0) + (c.sending || 0)
                            const pct = planned > 0 ? (sent / planned) * 100 : 0
                            // A draft/ready campaign with zero rows at all hasn't had its
                            // audience built yet — "0/0" reads as broken, so say that plainly
                            // instead of drawing an empty progress bar.
                            const audienceNotBuilt = Object.keys(c).length === 0 && ['draft', 'ready'].includes(row.status)
                            return (
                                <TableRow key={row.id} hover>
                                    <TableCell>
                                        <Typography variant='body2'>{row.name}</Typography>
                                        {row.dry_run && (
                                            <Typography variant='caption' color='text.secondary'>
                                                dry run
                                            </Typography>
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        <Chip size='small' label={row.status} color={STATUS_COLOR[row.status] || 'default'} />
                                        {(c.uncertain || 0) > 0 && (
                                            <Chip size='small' label='perlu ditinjau' color='warning' sx={{ ml: 0.5 }} />
                                        )}
                                    </TableCell>
                                    <TableCell sx={{ minWidth: 160 }}>
                                        {audienceNotBuilt ? (
                                            <Typography variant='caption' color='text.secondary'>
                                                Audiens belum dibangun
                                            </Typography>
                                        ) : (
                                            <>
                                                <LinearProgress
                                                    variant='determinate'
                                                    value={pct}
                                                    sx={{ height: 6, borderRadius: 1, mb: 0.5 }}
                                                />
                                                <Typography variant='caption' color='text.secondary'>
                                                    {sent}/{planned}
                                                    {(c.failed || 0) > 0 && ` · ${c.failed} gagal`}
                                                    {(c.skipped || 0) > 0 && ` · ${c.skipped} dilewati`}
                                                </Typography>
                                            </>
                                        )}
                                    </TableCell>
                                    <TableCell sx={{ color: 'text.secondary' }}>
                                        {row.scheduled_at ? new Date(row.scheduled_at).toLocaleString('id-ID') : '—'}
                                    </TableCell>
                                    <TableCell align='right'>
                                        <Stack direction='row' spacing={0.5} justifyContent='flex-end'>
                                            <Tooltip title='Detail'>
                                                <IconButton
                                                    size='small'
                                                    sx={{ color: 'text.secondary' }}
                                                    onClick={() => setDetailId(row.id)}
                                                >
                                                    <IconEye size={16} />
                                                </IconButton>
                                            </Tooltip>
                                            {['draft', 'ready', 'scheduled', 'paused'].includes(row.status) && (
                                                <Tooltip title='Edit'>
                                                    <IconButton size='small' sx={{ color: 'text.secondary' }} onClick={() => openEdit(row)}>
                                                        <IconEdit size={16} />
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                            {['draft', 'ready'].includes(row.status) && (
                                                <Tooltip title='Bangun audiens & jalankan'>
                                                    <IconButton
                                                        size='small'
                                                        sx={{ color: 'text.secondary' }}
                                                        onClick={() => buildAndStart(row)}
                                                    >
                                                        <IconPlayerPlay size={16} />
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                            {row.status === 'paused' && (
                                                <Tooltip title='Lanjutkan (tanpa membangun ulang audiens)'>
                                                    <IconButton
                                                        size='small'
                                                        sx={{ color: 'text.secondary' }}
                                                        onClick={() => resumePaused(row)}
                                                    >
                                                        <IconPlayerPlay size={16} />
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                            {row.status !== 'running' && (
                                                <Tooltip title='Hapus'>
                                                    <IconButton size='small' sx={{ color: 'text.secondary' }} onClick={() => remove(row)}>
                                                        <IconTrash size={16} />
                                                    </IconButton>
                                                </Tooltip>
                                            )}
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
                    onPageChange={(e, p) => setPage(p)}
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

            <Box sx={{ mt: 2 }}>
                <Typography variant='caption' color='text.secondary'>
                    Pengiriman dibatasi otomatis (jeda acak, jeda batch, batas harian, jam kerja) untuk melindungi nomor WhatsApp dari
                    pemblokiran. Customer yang membalas <b>STOP</b> otomatis dikeluarkan dari semua broadcast berikutnya.
                </Typography>
            </Box>

            <CampaignDialog open={dialogOpen} campaign={editing} onClose={() => setDialogOpen(false)} onSaved={() => refresh()} />
            <CampaignDetail id={detailId} open={!!detailId} onClose={() => setDetailId(null)} onChanged={refresh} />
        </MainCard>
    )
}

export default CRMBroadcast
