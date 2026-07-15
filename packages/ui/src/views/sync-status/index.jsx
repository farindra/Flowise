import { useState } from 'react'
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    IconButton,
    Paper,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography
} from '@mui/material'
import { IconChevronDown, IconChevronUp, IconRefresh, IconAlertCircle, IconInfoCircle, IconAlertTriangle, IconTrash, IconX } from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/sync-status'

const toWIB = (iso) => {
    if (!iso) return ''
    try {
        return new Intl.DateTimeFormat('id-ID', {
            timeZone: 'Asia/Jakarta',
            year: 'numeric', month: '2-digit', day: '2-digit',
            hour: '2-digit', minute: '2-digit', second: '2-digit',
            hour12: false
        }).format(new Date(iso)).replace(/(\d+)\/(\d+)\/(\d+),?/, '$3-$2-$1')
    } catch {
        return iso
    }
}

const levelColor = { ERROR: 'error', WARN: 'warning', INFO: 'success' }
const levelIcon = {
    ERROR: <IconAlertCircle size={14} />,
    WARN: <IconAlertTriangle size={14} />,
    INFO: <IconInfoCircle size={14} />
}

function LogRow({ row, idx }) {
    const [open, setOpen] = useState(false)
    return (
        <>
            <TableRow hover sx={{ cursor: 'pointer' }} onClick={() => setOpen(!open)}>
                <TableCell sx={{ width: 36, py: 0.5 }}>
                    <IconButton size='small'>{open ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}</IconButton>
                </TableCell>
                <TableCell sx={{ py: 0.5, width: 70 }}>
                    <Chip
                        size='small'
                        label={row.level}
                        color={levelColor[row.level] || 'default'}
                        icon={levelIcon[row.level]}
                        sx={{ fontWeight: 700, fontSize: 10 }}
                    />
                </TableCell>
                <TableCell sx={{ py: 0.5, whiteSpace: 'nowrap', width: 150 }}>
                    <Typography variant='caption' color='text.disabled' sx={{ fontFamily: 'monospace' }}>
                        {toWIB(row.time)}
                    </Typography>
                </TableCell>
                <TableCell sx={{ py: 0.5 }}>
                    <Typography variant='body2' noWrap sx={{ fontFamily: 'monospace', fontSize: 12, color: 'text.secondary', maxWidth: 500 }}>
                        {row.message}
                    </Typography>
                </TableCell>
            </TableRow>
            <TableRow>
                <TableCell colSpan={4} sx={{ py: 0, border: 0 }}>
                    <Collapse in={open} unmountOnExit>
                        <Box sx={{ mx: 1, mb: 1, p: 1.5, bgcolor: 'background.default', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                            <Typography component='pre' sx={{ fontFamily: 'monospace', fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-all', m: 0, color: 'text.secondary' }}>
                                {row.message}
                            </Typography>
                        </Box>
                    </Collapse>
                </TableCell>
            </TableRow>
        </>
    )
}

export default function SyncStatus() {
    const [status, setStatus] = useState(null)
    const [logs, setLogs] = useState(null)
    const [loading, setLoading] = useState(false)
    const [clearing, setClearing] = useState(false)
    const [error, setError] = useState(null)
    const [clearOpen, setClearOpen] = useState(false)
    const [clearMsg, setClearMsg] = useState(null)

    const fetchAll = async () => {
        setLoading(true)
        setError(null)
        try {
            const [statusRes, logsRes] = await Promise.all([
                fetch(`${API}/status`).then((r) => r.json()),
                fetch(`${API}/logs?lines=200`).then((r) => r.json())
            ])
            if (statusRes.error) throw new Error(statusRes.error)
            setStatus(statusRes)
            setLogs(logsRes)
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }

    const clearLogs = async () => {
        setClearOpen(false)
        setClearing(true)
        setClearMsg(null)
        try {
            const res = await fetch(`${API}/clear-logs`, { method: 'POST' })
            const data = await res.json()
            if (data.error) throw new Error(data.error)
            setLogs(null)
            setClearMsg('Log berhasil dihapus')
        } catch (e) {
            setClearMsg('Gagal hapus log: ' + e.message)
        } finally {
            setClearing(false)
        }
    }

    const lastSync = logs?.logs?.find((l) => l.message.includes('sync completed') || l.message.includes('sync failed'))
    const hasError = logs?.logs?.some((l) => l.level === 'ERROR')

    return (
        <MainCard title='Sync Status — Meilisearch'>
            <Stack spacing={2}>
                <Stack direction='row' spacing={1.5} alignItems='center' flexWrap='wrap'>
                    <Button
                        variant='contained'
                        onClick={fetchAll}
                        disabled={loading || clearing}
                        startIcon={loading ? <CircularProgress size={14} color='inherit' /> : <IconRefresh size={16} />}
                    >
                        {loading ? 'Memuat...' : 'Cek Status'}
                    </Button>
                    <Button
                        variant='outlined'
                        color='error'
                        onClick={() => setClearOpen(true)}
                        disabled={loading || clearing}
                        startIcon={clearing ? <CircularProgress size={14} color='inherit' /> : <IconTrash size={16} />}
                    >
                        {clearing ? 'Menghapus...' : 'Hapus Log'}
                    </Button>
                    <Typography variant='caption' color='text.disabled'>
                        Log ditampilkan 7 hari terakhir, interval sync 30 menit
                    </Typography>
                </Stack>

                <Dialog open={clearOpen} onClose={() => setClearOpen(false)}>
                    <DialogTitle>Hapus Log Sync-Indexer?</DialogTitle>
                    <DialogContent>
                        <DialogContentText>
                            Semua log container ob-sync-indexer akan dihapus permanen. Lanjutkan?
                        </DialogContentText>
                    </DialogContent>
                    <DialogActions>
                        <Button onClick={() => setClearOpen(false)}>Batal</Button>
                        <Button onClick={clearLogs} color='error' variant='contained'>Hapus</Button>
                    </DialogActions>
                </Dialog>

                {error && <Alert severity='error'>{error}</Alert>}
                {clearMsg && (
                    <Alert
                        severity={clearMsg.startsWith('Gagal') ? 'error' : 'success'}
                        action={
                            <IconButton size='small' color='inherit' onClick={() => setClearMsg(null)}>
                                <IconX size={16} />
                            </IconButton>
                        }
                    >
                        {clearMsg}
                    </Alert>
                )}

                {status && (
                    <Stack direction='row' spacing={2} flexWrap='wrap'>
                        <Paper variant='outlined' sx={{ p: 2, minWidth: 180 }}>
                            <Typography variant='caption' color='text.secondary' display='block' mb={0.5}>
                                Container
                            </Typography>
                            <Chip
                                label={status.running ? 'Running' : 'Stopped'}
                                color={status.running ? 'success' : 'error'}
                                size='small'
                                sx={{ fontWeight: 700 }}
                            />
                            <Typography variant='caption' color='text.disabled' display='block' mt={0.5}>
                                {status.status}
                            </Typography>
                        </Paper>
                        <Paper variant='outlined' sx={{ p: 2, minWidth: 200 }}>
                            <Typography variant='caption' color='text.secondary' display='block' mb={0.5}>
                                Container Start
                            </Typography>
                            <Typography variant='body2' sx={{ fontFamily: 'monospace', fontSize: 12 }}>
                                {toWIB(status.startedAt)}
                            </Typography>
                        </Paper>
                        {lastSync && (
                            <Paper variant='outlined' sx={{ p: 2, minWidth: 250 }}>
                                <Typography variant='caption' color='text.secondary' display='block' mb={0.5}>
                                    Sync Terakhir
                                </Typography>
                                <Chip
                                    size='small'
                                    label={lastSync.message.includes('completed') ? 'Berhasil' : 'Gagal'}
                                    color={lastSync.message.includes('completed') ? 'success' : 'error'}
                                    sx={{ fontWeight: 700, mb: 0.5 }}
                                />
                                <Typography variant='caption' color='text.disabled' display='block'>
                                    {toWIB(lastSync.time)}
                                </Typography>
                            </Paper>
                        )}
                        {hasError && (
                            <Paper variant='outlined' sx={{ p: 2, bgcolor: 'error.light', minWidth: 200 }}>
                                <Typography variant='caption' color='error.dark' display='block' mb={0.5}>
                                    Perhatian
                                </Typography>
                                <Typography variant='body2' color='error.dark' sx={{ fontSize: 12 }}>
                                    Ada error dalam log — cek detail di bawah
                                </Typography>
                            </Paper>
                        )}
                    </Stack>
                )}

                {logs && logs.total > 0 && (
                    <Box>
                        <Typography variant='caption' color='text.secondary' sx={{ mb: 1, display: 'block' }}>
                            {logs.total} baris log terbaru
                        </Typography>
                        <TableContainer component={Paper} variant='outlined' sx={{ maxHeight: 500 }}>
                            <Table size='small' stickyHeader>
                                <TableHead>
                                    <TableRow>
                                        <TableCell sx={{ width: 36 }} />
                                        <TableCell sx={{ width: 70 }}>Level</TableCell>
                                        <TableCell sx={{ width: 150 }}>Waktu (WIB)</TableCell>
                                        <TableCell>Pesan</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {logs.logs.map((row, i) => (
                                        <LogRow key={i} row={row} idx={i} />
                                    ))}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    </Box>
                )}
            </Stack>
        </MainCard>
    )
}
