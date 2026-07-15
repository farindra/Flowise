import { useState } from 'react'
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
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
import { IconChevronDown, IconChevronUp, IconRefresh, IconAlertCircle, IconInfoCircle, IconAlertTriangle } from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/sync-status'

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
                        {(row.time || '').replace('T', ' ').replace('Z', '').slice(0, 19)}
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
    const [error, setError] = useState(null)

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

    const lastSync = logs?.logs?.find((l) => l.message.includes('sync completed') || l.message.includes('sync failed'))
    const hasError = logs?.logs?.some((l) => l.level === 'ERROR')

    return (
        <MainCard title='Sync Status — Meilisearch'>
            <Stack spacing={2}>
                <Stack direction='row' spacing={1.5} alignItems='center'>
                    <Button
                        variant='contained'
                        onClick={fetchAll}
                        disabled={loading}
                        startIcon={loading ? <CircularProgress size={14} color='inherit' /> : <IconRefresh size={16} />}
                    >
                        {loading ? 'Memuat...' : 'Cek Status'}
                    </Button>
                    <Typography variant='caption' color='text.disabled'>
                        Klik untuk melihat log terbaru sync-indexer → Meilisearch (interval 30 menit)
                    </Typography>
                </Stack>

                {error && <Alert severity='error'>{error}</Alert>}

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
                                {(status.startedAt || '').replace('T', ' ').replace('Z', '').slice(0, 19)}
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
                                    {(lastSync.time || '').replace('T', ' ').replace('Z', '').slice(0, 19)}
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
                                        <TableCell sx={{ width: 150 }}>Waktu (UTC)</TableCell>
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
