import { useState, useEffect, useCallback } from 'react'
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
    Divider,
    FormControlLabel,
    Grid,
    IconButton,
    Paper,
    Slider,
    Stack,
    Switch,
    Tab,
    Tabs,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography
} from '@mui/material'
import {
    IconChevronDown,
    IconChevronUp,
    IconRefresh,
    IconAlertCircle,
    IconInfoCircle,
    IconAlertTriangle,
    IconTrash,
    IconX,
    IconPlayerPlay,
    IconClock,
    IconCheck,
    IconSettings
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/sync-status'

// ── Helpers ───────────────────────────────────────────────────────────────────

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

const fmtInterval = (mins) => {
    if (mins < 60) return `${mins} menit`
    const h = Math.floor(mins / 60)
    const m = mins % 60
    return m === 0 ? `${h} jam` : `${h}j ${m}m`
}

const levelColor = { ERROR: 'error', WARN: 'warning', INFO: 'success' }
const levelIcon = {
    ERROR: <IconAlertCircle size={14} />,
    WARN: <IconAlertTriangle size={14} />,
    INFO: <IconInfoCircle size={14} />
}

// ── Log row ───────────────────────────────────────────────────────────────────

function LogRow({ row }) {
    const [open, setOpen] = useState(false)
    return (
        <>
            <TableRow hover sx={{ cursor: 'pointer' }} onClick={() => setOpen(!open)}>
                <TableCell sx={{ width: 36, py: 0.5 }}>
                    <IconButton size='small'>{open ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}</IconButton>
                </TableCell>
                <TableCell sx={{ py: 0.5, width: 70 }}>
                    <Chip size='small' label={row.level} color={levelColor[row.level] || 'default'} icon={levelIcon[row.level]} sx={{ fontWeight: 700, fontSize: 10 }} />
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

// ── Schedule editor ───────────────────────────────────────────────────────────

const HOUR_OPTIONS = Array.from({ length: 24 }, (_, i) => i)

function ScheduleEditor({ serviceId, schedule, onSave }) {
    const [mode, setMode] = useState(schedule?.mode || 'interval')
    const [intervalMinutes, setIntervalMinutes] = useState(schedule?.intervalMinutes || 30)
    const [hours, setHours] = useState(schedule?.hours || [])
    const [saving, setSaving] = useState(false)
    const [msg, setMsg] = useState(null)

    const toggleHour = (h) => setHours((prev) => (prev.includes(h) ? prev.filter((x) => x !== h) : [...prev, h].sort((a, b) => a - b)))

    const save = async () => {
        setSaving(true)
        setMsg(null)
        try {
            const res = await fetch(`${API}/services/${serviceId}/schedule`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ mode, intervalMinutes, hours })
            })
            const data = await res.json()
            if (data.error) throw new Error(data.error)
            setMsg({ ok: true, text: data.message })
            onSave?.({ mode, intervalMinutes, hours })
        } catch (e) {
            setMsg({ ok: false, text: e.message })
        } finally {
            setSaving(false)
        }
    }

    // Slider marks
    const marks = [5, 10, 15, 30, 60, 120, 180, 360, 720, 1440].map((v) => ({ value: v, label: fmtInterval(v) }))

    return (
        <Box sx={{ pt: 1.5, pb: 0.5 }}>
            <Stack spacing={2}>
                <Stack direction='row' spacing={1} alignItems='center'>
                    <Typography variant='caption' color='text.secondary' sx={{ minWidth: 80 }}>Mode jadwal</Typography>
                    <FormControlLabel
                        control={<Switch size='small' checked={mode === 'hours'} onChange={(e) => setMode(e.target.checked ? 'hours' : 'interval')} />}
                        label={<Typography variant='caption'>{mode === 'hours' ? 'Jam tertentu (WIB)' : 'Interval'}</Typography>}
                    />
                </Stack>

                {mode === 'interval' ? (
                    <Box sx={{ px: 1 }}>
                        <Typography variant='caption' color='text.secondary' gutterBottom display='block'>
                            Interval: <b>{fmtInterval(intervalMinutes)}</b>
                        </Typography>
                        <Slider
                            value={intervalMinutes}
                            onChange={(_, v) => setIntervalMinutes(v)}
                            min={5}
                            max={1440}
                            step={null}
                            marks={marks}
                            valueLabelDisplay='auto'
                            valueLabelFormat={fmtInterval}
                            size='small'
                        />
                    </Box>
                ) : (
                    <Box>
                        <Typography variant='caption' color='text.secondary' gutterBottom display='block'>
                            Pilih jam sync (WIB) — {hours.length === 0 ? 'belum dipilih' : hours.map((h) => `${String(h).padStart(2, '0')}:00`).join(', ')}
                        </Typography>
                        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
                            {HOUR_OPTIONS.map((h) => (
                                <Chip
                                    key={h}
                                    label={`${String(h).padStart(2, '0')}:00`}
                                    size='small'
                                    variant={hours.includes(h) ? 'filled' : 'outlined'}
                                    color={hours.includes(h) ? 'primary' : 'default'}
                                    onClick={() => toggleHour(h)}
                                    sx={{ fontFamily: 'monospace', fontSize: 11, cursor: 'pointer' }}
                                />
                            ))}
                        </Box>
                    </Box>
                )}

                {msg && (
                    <Alert severity={msg.ok ? 'success' : 'error'} sx={{ py: 0.5 }}>
                        {msg.text}
                    </Alert>
                )}

                <Button
                    size='small'
                    variant='contained'
                    onClick={save}
                    disabled={saving || (mode === 'hours' && hours.length === 0)}
                    startIcon={saving ? <CircularProgress size={12} color='inherit' /> : <IconCheck size={14} />}
                    sx={{ alignSelf: 'flex-start' }}
                >
                    {saving ? 'Menyimpan...' : 'Simpan Jadwal'}
                </Button>
            </Stack>
        </Box>
    )
}

// ── Service card ──────────────────────────────────────────────────────────────

function ServiceCard({ service, onRefresh }) {
    const [triggering, setTriggering] = useState(false)
    const [triggerMsg, setTriggerMsg] = useState(null)
    const [scheduleOpen, setScheduleOpen] = useState(false)
    const [schedule, setSchedule] = useState(service.schedule)
    const [errDismissed, setErrDismissed] = useState(false)
    const [togglingEnabled, setTogglingEnabled] = useState(false)

    const trigger = async () => {
        setTriggering(true)
        setTriggerMsg(null)
        try {
            const res = await fetch(`${API}/services/${service.id}/trigger`, { method: 'POST' })
            const data = await res.json()
            if (data.error) throw new Error(data.error)
            const result = data.result
            const status = result?.status || 'triggered'
            setTriggerMsg({ ok: true, text: status === 'already running' ? 'Sync sedang berjalan' : 'Sync berhasil dimulai' })
            setTimeout(() => { setTriggerMsg(null); onRefresh?.() }, 3000)
        } catch (e) {
            setTriggerMsg({ ok: false, text: 'Gagal: ' + e.message })
        } finally {
            setTriggering(false)
        }
    }

    const toggleEnabled = async () => {
        const newEnabled = !(schedule?.enabled !== false)
        setTogglingEnabled(true)
        try {
            const res = await fetch(`${API}/services/${service.id}/schedule`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ...schedule, enabled: newEnabled })
            })
            const data = await res.json()
            if (data.error) throw new Error(data.error)
            setSchedule((prev) => ({ ...prev, enabled: newEnabled }))
        } catch (e) {
            setTriggerMsg({ ok: false, text: 'Gagal ubah status: ' + e.message })
        } finally {
            setTogglingEnabled(false)
        }
    }

    const sched = schedule
    const isEnabled = sched?.enabled !== false
    const schedLabel = !isEnabled
        ? 'Nonaktif'
        : sched
        ? sched.mode === 'hours' && sched.hours?.length > 0
            ? `Jam ${sched.hours.map((h) => `${String(h).padStart(2, '0')}:00`).join(', ')} WIB`
            : `Setiap ${fmtInterval(sched.intervalMinutes)}`
        : '-'

    const hs = service.httpStatus
    const lastSyncTime = hs?.last_run ? hs.last_run + ' WIB' : null
    const lastSyncErrs = hs?.last_errors?.filter(Boolean) || []
    const httpRunning = hs?.running === true

    return (
        <Paper variant='outlined' sx={{ p: 2, borderRadius: 2, position: 'relative' }}>
            <Stack spacing={1.5}>
                {/* Header row */}
                <Stack direction='row' justifyContent='space-between' alignItems='flex-start' spacing={1}>
                    <Box>
                        <Typography variant='subtitle2' fontWeight={700}>{service.label}</Typography>
                        <Typography variant='caption' color='text.secondary'>{service.description}</Typography>
                    </Box>
                    <Chip
                        label={service.running ? 'Running' : 'Stopped'}
                        color={service.running ? 'success' : 'error'}
                        size='small'
                        sx={{ fontWeight: 700, mt: 0.5 }}
                    />
                </Stack>

                {/* Info row */}
                <Stack direction='row' spacing={2} flexWrap='wrap'>
                    <Box>
                        <Typography variant='caption' color='text.disabled'>Index</Typography>
                        <Typography variant='caption' display='block' sx={{ fontFamily: 'monospace', fontSize: 11 }}>
                            {service.index}
                        </Typography>
                    </Box>
                    {service.startedAt && (
                        <Box>
                            <Typography variant='caption' color='text.disabled'>Uptime sejak</Typography>
                            <Typography variant='caption' display='block' sx={{ fontFamily: 'monospace', fontSize: 11 }}>
                                {toWIB(service.startedAt)}
                            </Typography>
                        </Box>
                    )}
                    {lastSyncTime && (
                        <Box>
                            <Typography variant='caption' color='text.disabled'>Sync terakhir</Typography>
                            <Typography variant='caption' display='block' sx={{ fontFamily: 'monospace', fontSize: 11 }}>
                                {lastSyncTime}
                            </Typography>
                        </Box>
                    )}
                    {hs && (
                        <Box>
                            <Typography variant='caption' color='text.disabled'>Total sync</Typography>
                            <Typography variant='caption' display='block' sx={{ fontFamily: 'monospace', fontSize: 11 }}>
                                {hs.total_runs ?? '-'}x ({hs.last_dur || '-'})
                            </Typography>
                        </Box>
                    )}
                </Stack>

                {hs && httpRunning && (
                    <Alert severity='info' sx={{ py: 0.5, fontSize: 12 }}>
                        Sync sedang berjalan: {hs.current_progress?.elapsed} —{' '}
                        {hs.current_progress?.products_indexed} produk
                    </Alert>
                )}

                {lastSyncErrs.length > 0 && !errDismissed && (
                    <Alert
                        severity='warning'
                        sx={{ py: 0.5, fontSize: 11 }}
                        action={
                            <IconButton size='small' color='inherit' onClick={() => setErrDismissed(true)}>
                                <IconX size={14} />
                            </IconButton>
                        }
                    >
                        Error terakhir: {lastSyncErrs[0]}
                    </Alert>
                )}

                {triggerMsg && (
                    <Alert severity={triggerMsg.ok ? 'success' : 'error'} sx={{ py: 0.5, fontSize: 12 }}>
                        {triggerMsg.text}
                    </Alert>
                )}

                {/* Actions */}
                <Stack direction='row' spacing={1} alignItems='center' flexWrap='wrap'>
                    <Button
                        size='small'
                        variant='contained'
                        onClick={trigger}
                        disabled={triggering}
                        startIcon={triggering ? <CircularProgress size={12} color='inherit' /> : <IconPlayerPlay size={14} />}
                    >
                        {triggering ? 'Memulai...' : 'Sync Sekarang'}
                    </Button>
                    <Tooltip title={isEnabled ? 'Nonaktifkan auto-sync' : 'Aktifkan auto-sync'}>
                        <FormControlLabel
                            sx={{ ml: 0.5, mr: 0 }}
                            control={
                                <Switch
                                    size='small'
                                    checked={isEnabled}
                                    onChange={toggleEnabled}
                                    disabled={togglingEnabled}
                                    color='success'
                                />
                            }
                            label={
                                <Typography variant='caption' color={isEnabled ? 'success.main' : 'text.disabled'}>
                                    {togglingEnabled ? '...' : isEnabled ? 'Auto' : 'Off'}
                                </Typography>
                            }
                        />
                    </Tooltip>
                    <Tooltip title='Atur jadwal'>
                        <Button
                            size='small'
                            variant='outlined'
                            color={isEnabled ? 'primary' : 'inherit'}
                            onClick={() => setScheduleOpen(!scheduleOpen)}
                            startIcon={<IconSettings size={14} />}
                            endIcon={scheduleOpen ? <IconChevronUp size={12} /> : <IconChevronDown size={12} />}
                        >
                            {schedLabel}
                        </Button>
                    </Tooltip>
                </Stack>

                {/* Schedule editor */}
                <Collapse in={scheduleOpen} unmountOnExit>
                    <Divider sx={{ my: 0.5 }} />
                    <ScheduleEditor
                        serviceId={service.id}
                        schedule={schedule}
                        onSave={(newSched) => {
                            setSchedule(newSched)
                            setTimeout(() => setScheduleOpen(false), 1500)
                        }}
                    />
                </Collapse>
            </Stack>
        </Paper>
    )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function SyncStatus() {
    const [services, setServices] = useState(null)
    const [servicesLoading, setServicesLoading] = useState(false)
    const [servicesError, setServicesError] = useState(null)

    const [logs, setLogs] = useState(null)
    const [logsLoading, setLogsLoading] = useState(false)
    const [logsError, setLogsError] = useState(null)
    const [logService, setLogService] = useState('ob-sync-indexer')

    const [clearing, setClearing] = useState(false)
    const [clearOpen, setClearOpen] = useState(false)
    const [clearMsg, setClearMsg] = useState(null)

    const fetchServices = useCallback(async () => {
        setServicesLoading(true)
        setServicesError(null)
        try {
            const data = await fetch(`${API}/services`).then((r) => r.json())
            if (data.error) throw new Error(data.error)
            setServices(data)
        } catch (e) {
            setServicesError(e.message)
        } finally {
            setServicesLoading(false)
        }
    }, [])

    const fetchLogs = useCallback(async (serviceId) => {
        const svc = serviceId || logService
        setLogsLoading(true)
        setLogsError(null)
        setLogs(null)
        try {
            const data = await fetch(`${API}/services/${svc}/logs?lines=150`).then((r) => r.json())
            if (data.error) throw new Error(data.error)
            setLogs(data)
        } catch (e) {
            setLogsError(e.message)
        } finally {
            setLogsLoading(false)
        }
    }, [logService])

    useEffect(() => {
        fetchServices()
    }, [fetchServices])

    const clearLogs = async () => {
        setClearOpen(false)
        setClearing(true)
        setClearMsg(null)
        try {
            const res = await fetch(`${API}/services/${logService}/clear-logs`, { method: 'POST' })
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

    return (
        <MainCard title='Sync Status — Meilisearch'>
            <Stack spacing={3}>

                {/* ── Services section ── */}
                <Box>
                    <Stack direction='row' justifyContent='space-between' alignItems='center' mb={1.5}>
                        <Typography variant='h6' fontWeight={600}>Layanan Sync</Typography>
                        <Button
                            size='small'
                            variant='outlined'
                            onClick={fetchServices}
                            disabled={servicesLoading}
                            startIcon={servicesLoading ? <CircularProgress size={12} color='inherit' /> : <IconRefresh size={14} />}
                        >
                            {servicesLoading ? 'Memuat...' : 'Refresh'}
                        </Button>
                    </Stack>

                    {servicesError && <Alert severity='error' sx={{ mb: 1.5 }}>{servicesError}</Alert>}

                    {services ? (
                        <Grid container spacing={2}>
                            {services.map((svc) => (
                                <Grid item xs={12} md={6} key={svc.id}>
                                    <ServiceCard service={svc} onRefresh={fetchServices} />
                                </Grid>
                            ))}
                        </Grid>
                    ) : (
                        !servicesLoading && (
                            <Typography variant='body2' color='text.secondary'>Klik Refresh untuk melihat status layanan.</Typography>
                        )
                    )}
                    {servicesLoading && !services && (
                        <Box display='flex' alignItems='center' gap={1} py={2}>
                            <CircularProgress size={18} />
                            <Typography variant='body2' color='text.secondary'>Memuat status layanan...</Typography>
                        </Box>
                    )}
                </Box>

                <Divider />

                {/* ── Logs section ── */}
                <Box>
                    <Typography variant='h6' fontWeight={600} mb={1.5}>Log Service</Typography>

                    {/* Service tabs */}
                    <Tabs
                        value={logService}
                        onChange={(_, v) => { setLogService(v); setLogs(null); setLogsError(null) }}
                        variant='scrollable'
                        scrollButtons='auto'
                        sx={{ mb: 1.5, borderBottom: 1, borderColor: 'divider' }}
                    >
                        <Tab value='ob-sync-indexer' label='Trade Sync (OB)' />
                        <Tab value='go-index' label='PrestaShop Indexer' />
                        <Tab value='go-jurnal-sync' label='Jurnal Sync OB' />
                        <Tab value='go-jurnal-sync-sbb' label='Jurnal Sync SBB' />
                    </Tabs>

                    <Stack direction='row' spacing={1.5} alignItems='center' flexWrap='wrap' mb={1.5}>
                        <Button
                            variant='contained'
                            onClick={() => fetchLogs(logService)}
                            disabled={logsLoading || clearing}
                            startIcon={logsLoading ? <CircularProgress size={14} color='inherit' /> : <IconRefresh size={16} />}
                        >
                            {logsLoading ? 'Memuat...' : 'Lihat Log'}
                        </Button>
                        <Button
                            variant='outlined'
                            color='error'
                            onClick={() => setClearOpen(true)}
                            disabled={logsLoading || clearing}
                            startIcon={clearing ? <CircularProgress size={14} color='inherit' /> : <IconTrash size={16} />}
                        >
                            {clearing ? 'Menghapus...' : 'Hapus Log'}
                        </Button>
                        <Typography variant='caption' color='text.disabled'>
                            Log 7 hari terakhir, maks 150 baris
                        </Typography>
                    </Stack>

                    {clearMsg && (
                        <Alert
                            severity={clearMsg.startsWith('Gagal') ? 'error' : 'success'}
                            sx={{ mb: 1.5 }}
                            action={
                                <IconButton size='small' color='inherit' onClick={() => setClearMsg(null)}>
                                    <IconX size={16} />
                                </IconButton>
                            }
                        >
                            {clearMsg}
                        </Alert>
                    )}

                    {logsError && <Alert severity='error' sx={{ mb: 1.5 }}>{logsError}</Alert>}

                    {logs && logs.total > 0 && (
                        <Box>
                            <Typography variant='caption' color='text.secondary' sx={{ mb: 1, display: 'block' }}>
                                {logs.total} baris log — <b>{logs.label}</b>
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
                                            <LogRow key={i} row={row} />
                                        ))}
                                    </TableBody>
                                </Table>
                            </TableContainer>
                        </Box>
                    )}

                    {logs && logs.total === 0 && (
                        <Typography variant='body2' color='text.secondary'>Tidak ada log dalam 7 hari terakhir.</Typography>
                    )}
                </Box>

                {/* ── Confirm dialog ── */}
                <Dialog open={clearOpen} onClose={() => setClearOpen(false)}>
                    <DialogTitle>Hapus Log {logService}?</DialogTitle>
                    <DialogContent>
                        <DialogContentText>Semua log <b>{logService}</b> akan dihapus permanen. Lanjutkan?</DialogContentText>
                    </DialogContent>
                    <DialogActions>
                        <Button onClick={() => setClearOpen(false)}>Batal</Button>
                        <Button onClick={clearLogs} color='error' variant='contained'>Hapus</Button>
                    </DialogActions>
                </Dialog>
            </Stack>
        </MainCard>
    )
}
