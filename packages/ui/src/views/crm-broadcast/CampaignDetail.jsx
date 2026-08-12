import PropTypes from 'prop-types'
import { useCallback, useEffect, useRef, useState } from 'react'
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
    InputLabel,
    LinearProgress,
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
    Typography
} from '@mui/material'
import { IconDownload, IconPlayerPause, IconPlayerPlay, IconRefresh, IconX } from '@tabler/icons-react'
import broadcastApi from '@/api/crmbroadcast'

const TERMINAL = ['done', 'cancelled', 'draft', 'ready']

const STATUS_COLOR = {
    sent: 'success',
    failed: 'error',
    skipped: 'default',
    pending: 'info',
    sending: 'primary',
    uncertain: 'warning'
}

const CampaignDetail = ({ id, open, onClose, onChanged }) => {
    const [campaign, setCampaign] = useState(null)
    const [recipients, setRecipients] = useState([])
    const [total, setTotal] = useState(0)
    const [page, setPage] = useState(0)
    const [rowsPerPage, setRowsPerPage] = useState(25)
    const [statusFilter, setStatusFilter] = useState('')
    const [q, setQ] = useState('')
    const [error, setError] = useState('')
    const [building, setBuilding] = useState(false)
    const pollRef = useRef(null)

    const loadCampaign = useCallback(async () => {
        try {
            const { data } = await broadcastApi.getBroadcast(id)
            setCampaign(data)
            return data
        } catch (e) {
            setError(e.response?.data?.error || e.message)
            return null
        }
    }, [id])

    const loadRecipients = useCallback(async () => {
        try {
            const { data } = await broadcastApi.getRecipients(id, {
                page: page + 1,
                limit: rowsPerPage,
                status: statusFilter || undefined,
                q: q || undefined
            })
            setRecipients(data.items || [])
            setTotal(data.total || 0)
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }, [id, page, rowsPerPage, statusFilter, q])

    useEffect(() => {
        if (!open || !id) return
        loadCampaign()
        loadRecipients()
    }, [open, id, loadCampaign, loadRecipients])

    // Poll only while the campaign is actually moving, and always clear on
    // unmount — a multi-hour broadcast would otherwise leak a request per tick
    // long after the dialog is closed.
    const campaignStatus = campaign?.status
    useEffect(() => {
        // Only the status matters here: re-arming on every refetched object
        // would tear down and rebuild the interval three times a second.
        if (!open || !campaignStatus) return undefined
        if (TERMINAL.includes(campaignStatus)) return undefined

        pollRef.current = setInterval(async () => {
            const fresh = await loadCampaign()
            await loadRecipients()
            if (fresh && TERMINAL.includes(fresh.status) && pollRef.current) {
                clearInterval(pollRef.current)
                pollRef.current = null
            }
        }, 3000)

        return () => {
            if (pollRef.current) {
                clearInterval(pollRef.current)
                pollRef.current = null
            }
        }
    }, [open, campaignStatus, loadCampaign, loadRecipients])

    const act = async (fn) => {
        setError('')
        try {
            await fn()
            await loadCampaign()
            await loadRecipients()
            onChanged?.()
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    // Builds the recipient list from the saved audience spec, then starts
    // sending. Only valid for draft/ready — this REPLACES the recipient rows,
    // which for a paused campaign would wipe its sent/failed history and risk
    // re-sending to numbers it already reached. Paused must use resume().
    const buildAndStart = async () => {
        setError('')
        setBuilding(true)
        try {
            const { data } = await broadcastApi.buildAudience(id)
            for (;;) {
                await new Promise((r) => setTimeout(r, 800))
                const { data: job } = await broadcastApi.getBuildStatus(id, data.job_id)
                if (job.status === 'done') {
                    if (job.errors?.length) throw new Error(job.errors[0].error || 'gagal bangun audiens')
                    break
                }
            }
            await broadcastApi.startBroadcast(id)
            await loadCampaign()
            await loadRecipients()
            onChanged?.()
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        } finally {
            setBuilding(false)
        }
    }

    const doExport = async () => {
        try {
            const res = await broadcastApi.exportRecipients(id)
            const url = URL.createObjectURL(res.data)
            const a = document.createElement('a')
            a.href = url
            a.download = `broadcast-${id.slice(0, 8)}.xlsx`
            document.body.appendChild(a)
            a.click()
            a.remove()
            URL.revokeObjectURL(url)
        } catch (e) {
            setError(e.response?.data?.error || e.message)
        }
    }

    const counts = campaign?.counts || {}
    const sent = counts.sent || 0
    const failed = counts.failed || 0
    const uncertain = counts.uncertain || 0
    const skipped = counts.skipped || 0
    const pending = (counts.pending || 0) + (counts.sending || 0)
    const totalPlanned = sent + failed + uncertain + pending
    const progress = totalPlanned > 0 ? (sent / totalPlanned) * 100 : 0

    return (
        <Dialog open={open} onClose={onClose} fullWidth maxWidth='lg'>
            <DialogTitle>{campaign?.name || 'Detail Broadcast'}</DialogTitle>
            <DialogContent>
                {error && (
                    <Alert severity='error' sx={{ mb: 2 }}>
                        {error}
                    </Alert>
                )}

                {campaign && (
                    <Stack spacing={2}>
                        <Stack direction='row' spacing={1} alignItems='center' flexWrap='wrap' useFlexGap>
                            <Chip size='small' label={campaign.status} color={campaign.status === 'running' ? 'primary' : 'default'} />
                            {campaign.dry_run && <Chip size='small' label='dry run' color='info' variant='outlined' />}
                            <Box sx={{ flex: 1 }} />
                            {['draft', 'ready'].includes(campaign.status) && (
                                <Chip
                                    size='small'
                                    color='primary'
                                    label={building ? 'Membangun audiens...' : 'Bangun Audiens & Mulai'}
                                    clickable={!building}
                                    disabled={building}
                                    icon={building ? <CircularProgress size={12} color='inherit' /> : <IconPlayerPlay size={14} />}
                                    onClick={buildAndStart}
                                />
                            )}
                            {campaign.status === 'running' && (
                                <Chip
                                    size='small'
                                    variant='outlined'
                                    label='Jeda'
                                    clickable
                                    icon={<IconPlayerPause size={14} />}
                                    onClick={() => act(() => broadcastApi.pauseBroadcast(id))}
                                />
                            )}
                            {campaign.status === 'paused' && (
                                <Chip
                                    size='small'
                                    color='primary'
                                    label='Lanjutkan'
                                    clickable
                                    icon={<IconPlayerPlay size={14} />}
                                    onClick={() => act(() => broadcastApi.resumeBroadcast(id))}
                                />
                            )}
                            {['running', 'paused', 'scheduled'].includes(campaign.status) && (
                                <Chip
                                    size='small'
                                    color='error'
                                    variant='outlined'
                                    label='Batalkan'
                                    clickable
                                    icon={<IconX size={14} />}
                                    onClick={() => act(() => broadcastApi.cancelBroadcast(id))}
                                />
                            )}
                            <Chip
                                size='small'
                                variant='outlined'
                                label='Export'
                                clickable
                                icon={<IconDownload size={14} />}
                                onClick={doExport}
                            />
                            <Chip
                                size='small'
                                variant='outlined'
                                label='Refresh'
                                clickable
                                icon={<IconRefresh size={14} />}
                                onClick={() => act(async () => {})}
                            />
                        </Stack>

                        <Box>
                            <LinearProgress variant='determinate' value={progress} sx={{ height: 8, borderRadius: 1 }} />
                            <Typography variant='caption' color='text.secondary'>
                                {sent}/{totalPlanned} terkirim
                                {failed > 0 && ` · ${failed} gagal`}
                                {uncertain > 0 && ` · ${uncertain} perlu ditinjau`}
                                {skipped > 0 && ` · ${skipped} dilewati`}
                            </Typography>
                        </Box>

                        {campaign.last_error && <Alert severity='warning'>{campaign.last_error}</Alert>}

                        {uncertain > 0 && (
                            <Alert severity='warning'>
                                {uncertain} penerima statusnya tidak diketahui (service restart tepat saat mengirim). Pesan mungkin sudah
                                sampai atau belum — kirim ulang hanya kalau Anda yakin.
                                <Box sx={{ mt: 1 }}>
                                    <Chip
                                        size='small'
                                        color='warning'
                                        label='Kirim Ulang (tidak pasti)'
                                        clickable
                                        onClick={() => act(() => broadcastApi.retryBroadcast(id, 'uncertain'))}
                                    />
                                </Box>
                            </Alert>
                        )}

                        {failed > 0 && (
                            <Box>
                                <Chip
                                    size='small'
                                    color='error'
                                    variant='outlined'
                                    label={`Coba Ulang ${failed} yang Gagal`}
                                    clickable
                                    onClick={() => act(() => broadcastApi.retryBroadcast(id, 'failed'))}
                                />
                            </Box>
                        )}

                        <Divider />

                        <Stack direction='row' spacing={2}>
                            <FormControl size='small' sx={{ minWidth: 160 }}>
                                <InputLabel>Status</InputLabel>
                                <Select
                                    value={statusFilter}
                                    label='Status'
                                    onChange={(e) => {
                                        setStatusFilter(e.target.value)
                                        setPage(0)
                                    }}
                                >
                                    <MenuItem value=''>Semua</MenuItem>
                                    {['pending', 'sent', 'failed', 'skipped', 'uncertain'].map((s) => (
                                        <MenuItem key={s} value={s}>
                                            {s}
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            <TextField
                                size='small'
                                placeholder='Cari nama atau nomor...'
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter') setPage(0)
                                }}
                                sx={{ flex: 1 }}
                            />
                        </Stack>

                        <TableContainer component={Paper} variant='outlined'>
                            <Table size='small'>
                                <TableHead>
                                    <TableRow>
                                        <TableCell>Nama</TableCell>
                                        <TableCell>No WA</TableCell>
                                        <TableCell>Status</TableCell>
                                        <TableCell>Keterangan</TableCell>
                                        <TableCell>Percobaan</TableCell>
                                        <TableCell>Waktu Kirim</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {recipients.length === 0 && (
                                        <TableRow>
                                            <TableCell colSpan={6} align='center' sx={{ py: 4, color: 'text.secondary' }}>
                                                Belum ada penerima
                                            </TableCell>
                                        </TableRow>
                                    )}
                                    {recipients.map((r) => (
                                        <TableRow key={r.id}>
                                            <TableCell>{r.name || <span style={{ opacity: 0.6 }}>—</span>}</TableCell>
                                            <TableCell>{r.phone}</TableCell>
                                            <TableCell>
                                                <Chip size='small' label={r.status} color={STATUS_COLOR[r.status] || 'default'} />
                                            </TableCell>
                                            <TableCell sx={{ color: 'text.secondary', maxWidth: 280 }}>
                                                {r.skip_reason || r.last_error || '—'}
                                            </TableCell>
                                            <TableCell>{r.attempts}</TableCell>
                                            <TableCell sx={{ color: 'text.secondary' }}>
                                                {r.sent_at ? new Date(r.sent_at).toLocaleString('id-ID') : '—'}
                                            </TableCell>
                                        </TableRow>
                                    ))}
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

                        <Stack direction='row' justifyContent='flex-end'>
                            <Chip size='small' variant='outlined' label='Tutup' clickable onClick={onClose} />
                        </Stack>
                    </Stack>
                )}
            </DialogContent>
        </Dialog>
    )
}

CampaignDetail.propTypes = {
    id: PropTypes.string,
    open: PropTypes.bool.isRequired,
    onClose: PropTypes.func.isRequired,
    onChanged: PropTypes.func
}

export default CampaignDetail
