import PropTypes from 'prop-types'
import { useEffect, useState, useCallback } from 'react'
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    Divider,
    Grid,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Paper,
    Snackbar,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography
} from '@mui/material'
import {
    IconRefresh,
    IconAlertTriangle,
    IconCheck,
    IconDroplet,
    IconTemperature,
    IconWind,
    IconDotsVertical,
    IconBrandTelegram,
    IconMail
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/iot'
const REFRESH_INTERVAL = 30000

const SENSOR_META = {
    soil_humidity: { label: 'Kelembaban Tanah', icon: <IconDroplet size={14} />, unit: '%', color: '#0ea5e9' },
    air_humidity: { label: 'Kelembaban Udara', icon: <IconWind size={14} />, unit: '%', color: '#6366f1' },
    temperature: { label: 'Suhu Udara', icon: <IconTemperature size={14} />, unit: '°C', color: '#f97316' },
    motion: { label: 'Gerakan', icon: <IconAlertTriangle size={14} />, unit: '', color: '#ef4444' }
}

function ReadingBadge({ sensorType, reading }) {
    const meta = SENSOR_META[sensorType] || { label: sensorType, unit: '', color: '#888' }
    if (!reading) {
        return (
            <Stack direction='row' spacing={0.5} alignItems='center'>
                {meta.icon}
                <Typography variant='caption' color='text.disabled'>
                    —
                </Typography>
            </Stack>
        )
    }
    const age = Math.floor((Date.now() - new Date(reading.recorded_at)) / 60000)
    return (
        <Stack direction='row' spacing={0.5} alignItems='center'>
            {meta.icon}
            <Typography variant='caption' sx={{ color: meta.color, fontWeight: 600 }}>
                {reading.value}
                {meta.unit}
            </Typography>
            <Typography variant='caption' color='text.disabled'>
                ({age}m)
            </Typography>
        </Stack>
    )
}

ReadingBadge.propTypes = { sensorType: PropTypes.string, reading: PropTypes.object }

function ZoneCard({ zone }) {
    const [anchorEl, setAnchorEl] = useState(null)
    const [sending, setSending] = useState(null) // 'telegram'|'email'|null
    const [snack, setSnack] = useState(null)

    const statusColor = zone.status === 'alert' ? 'error' : zone.status === 'warning' ? 'warning' : 'success'
    const statusLabel = zone.status === 'alert' ? 'ALERT' : zone.status === 'warning' ? 'Warning' : 'Normal'

    const sendReport = async (channel) => {
        setAnchorEl(null)
        setSending(channel)
        try {
            const res = await fetch(`${API}/zones/${zone.id}/report`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ channels: [channel] })
            })
            const d = await res.json()
            const result = d.results?.[channel]
            if (result === 'sent')
                setSnack({ severity: 'success', msg: `Laporan terkirim ke ${channel === 'telegram' ? 'Telegram' : 'Email'}` })
            else if (result === 'not_configured') setSnack({ severity: 'warning', msg: 'Email belum dikonfigurasi di server' })
            else setSnack({ severity: 'error', msg: result || 'Gagal mengirim laporan' })
        } catch (e) {
            setSnack({ severity: 'error', msg: e.message })
        } finally {
            setSending(null)
        }
    }

    return (
        <>
            <Paper
                variant='outlined'
                sx={{
                    p: 2,
                    borderColor: zone.status === 'alert' ? 'error.main' : zone.status === 'warning' ? 'warning.main' : 'divider',
                    borderWidth: zone.status === 'alert' ? 2 : 1
                }}
            >
                <Stack direction='row' justifyContent='space-between' alignItems='flex-start' sx={{ mb: 1 }}>
                    <Box sx={{ flex: 1 }}>
                        <Typography variant='subtitle2' sx={{ fontWeight: 600 }}>
                            {zone.name}
                        </Typography>
                        <Typography variant='caption' color='text.secondary'>
                            {zone.description}
                        </Typography>
                    </Box>
                    <Stack direction='row' spacing={0.5} alignItems='center'>
                        <Chip size='small' label={statusLabel} color={statusColor} />
                        <IconButton
                            size='small'
                            onClick={(e) => setAnchorEl(e.currentTarget)}
                            sx={{ color: 'text.primary', bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
                        >
                            {sending ? <CircularProgress size={14} /> : <IconDotsVertical size={16} />}
                        </IconButton>
                    </Stack>
                </Stack>

                <Stack spacing={0.5}>
                    {['soil_humidity', 'air_humidity', 'temperature'].map((type) => (
                        <ReadingBadge key={type} sensorType={type} reading={zone.latest?.[type]} />
                    ))}
                    {zone.latest?.motion && <ReadingBadge sensorType='motion' reading={zone.latest.motion} />}
                </Stack>

                {zone.open_alerts > 0 && (
                    <Box sx={{ mt: 1 }}>
                        <Chip size='small' label={`${zone.open_alerts} alert aktif`} color='error' variant='outlined' />
                    </Box>
                )}
            </Paper>

            <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
                <Box sx={{ px: 2, py: 0.5 }}>
                    <Typography
                        variant='caption'
                        color='text.secondary'
                        sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5 }}
                    >
                        Kirim Laporan
                    </Typography>
                </Box>
                <MenuItem dense onClick={() => sendReport('telegram')}>
                    <ListItemIcon>
                        <IconBrandTelegram size={16} color='#2AABEE' />
                    </ListItemIcon>
                    <ListItemText primary='Telegram' primaryTypographyProps={{ variant: 'body2' }} />
                </MenuItem>
                <MenuItem dense onClick={() => sendReport('email')} disabled>
                    <ListItemIcon>
                        <IconMail size={16} />
                    </ListItemIcon>
                    <ListItemText primary='Email (belum aktif)' primaryTypographyProps={{ variant: 'body2' }} />
                </MenuItem>
            </Menu>

            <Snackbar
                open={Boolean(snack)}
                autoHideDuration={4000}
                onClose={() => setSnack(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            >
                <Alert onClose={() => setSnack(null)} severity={snack?.severity || 'info'} sx={{ width: '100%' }}>
                    {snack?.msg}
                </Alert>
            </Snackbar>
        </>
    )
}

ZoneCard.propTypes = { zone: PropTypes.object }

function AlertRow({ alert, onResolve }) {
    const [resolving, setResolving] = useState(false)

    const handleResolve = async () => {
        setResolving(true)
        try {
            await fetch(`${API}/alerts/${alert.id}/resolve`, { method: 'PUT' })
            onResolve()
        } finally {
            setResolving(false)
        }
    }

    const age = Math.floor((Date.now() - new Date(alert.created_at)) / 60000)
    const meta = SENSOR_META[alert.sensor_type] || {}

    return (
        <TableRow hover>
            <TableCell>
                <Stack direction='row' spacing={0.5} alignItems='center'>
                    <IconAlertTriangle size={14} color='#ef4444' />
                    <Typography variant='caption'>{alert.zone_name}</Typography>
                </Stack>
            </TableCell>
            <TableCell>
                <Typography variant='caption'>{meta.label || alert.sensor_type}</Typography>
            </TableCell>
            <TableCell>
                <Typography variant='caption' sx={{ fontWeight: 600, color: '#ef4444' }}>
                    {alert.value}
                    {meta.unit}
                </Typography>
                <Typography variant='caption' color='text.secondary'>
                    {' '}
                    ({alert.direction === 'below' ? 'min' : 'max'} {alert.threshold}
                    {meta.unit})
                </Typography>
            </TableCell>
            <TableCell>
                <Typography variant='caption' color='text.secondary'>
                    {age}m lalu
                </Typography>
            </TableCell>
            <TableCell>
                <IconButton size='small' color='success' onClick={handleResolve} disabled={resolving}>
                    {resolving ? <CircularProgress size={14} /> : <IconCheck size={14} />}
                </IconButton>
            </TableCell>
        </TableRow>
    )
}

AlertRow.propTypes = { alert: PropTypes.object, onResolve: PropTypes.func }

export default function IoTMonitor() {
    const [zones, setZones] = useState([])
    const [alerts, setAlerts] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)
    const [lastUpdate, setLastUpdate] = useState(null)

    const fetchAll = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const [zonesRes, alertsRes] = await Promise.all([fetch(`${API}/zones`), fetch(`${API}/alerts`)])
            setZones(await zonesRes.json())
            setAlerts(await alertsRes.json())
            setLastUpdate(new Date())
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        fetchAll()
        const t = setInterval(fetchAll, REFRESH_INTERVAL)
        return () => clearInterval(t)
    }, [fetchAll])

    const openAlerts = Array.isArray(alerts) ? alerts.filter((a) => !a.resolved) : []
    const totalAlert = Array.isArray(zones) ? zones.filter((z) => z.status === 'alert').length : 0

    return (
        <MainCard
            title='IoT Monitor — Kondisi Lahan AAMG'
            secondary={
                <Stack direction='row' spacing={1} alignItems='center'>
                    {lastUpdate && (
                        <Typography variant='caption' color='text.secondary'>
                            Update: {lastUpdate.toLocaleTimeString('id-ID')}
                        </Typography>
                    )}
                    <IconButton size='small' onClick={fetchAll} disabled={loading}>
                        {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                    </IconButton>
                </Stack>
            }
        >
            {error && (
                <Alert severity='error' sx={{ mb: 2 }}>
                    {error}: pastikan go-iot service berjalan (port 8084)
                </Alert>
            )}

            {/* Summary bar */}
            <Stack direction='row' spacing={2} sx={{ mb: 2 }}>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 100 }}>
                    <Typography variant='h5' color={totalAlert > 0 ? 'error.main' : 'success.main'} fontWeight={700}>
                        {totalAlert}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Zona Alert
                    </Typography>
                </Paper>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 100 }}>
                    <Typography variant='h5' color={openAlerts.length > 0 ? 'error.main' : 'success.main'} fontWeight={700}>
                        {openAlerts.length}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Alert Aktif
                    </Typography>
                </Paper>
                <Paper variant='outlined' sx={{ px: 2, py: 1, textAlign: 'center', minWidth: 100 }}>
                    <Typography variant='h5' color='primary.main' fontWeight={700}>
                        {Array.isArray(zones) ? zones.length : 0}
                    </Typography>
                    <Typography variant='caption' color='text.secondary'>
                        Total Zona
                    </Typography>
                </Paper>
            </Stack>

            {/* Zone cards */}
            <Typography variant='subtitle1' sx={{ mb: 1, fontWeight: 600 }}>
                Status Zona
            </Typography>
            <Grid container spacing={1.5} sx={{ mb: 3 }}>
                {(Array.isArray(zones) ? zones : []).map((zone) => (
                    <Grid item xs={12} sm={6} md={4} key={zone.id}>
                        <ZoneCard zone={zone} onRefresh={fetchAll} />
                    </Grid>
                ))}
                {!loading && zones.length === 0 && (
                    <Grid item xs={12}>
                        <Typography variant='body2' color='text.secondary' align='center' sx={{ py: 3 }}>
                            Belum ada data zona. Pastikan go-iot service berjalan.
                        </Typography>
                    </Grid>
                )}
            </Grid>

            <Divider sx={{ mb: 2 }} />

            {/* Alert log */}
            <Stack direction='row' justifyContent='space-between' alignItems='center' sx={{ mb: 1 }}>
                <Typography variant='subtitle1' sx={{ fontWeight: 600 }}>
                    Alert Aktif
                </Typography>
                {openAlerts.length === 0 && <Chip size='small' label='Semua normal' color='success' icon={<IconCheck size={12} />} />}
            </Stack>

            {openAlerts.length > 0 && (
                <TableContainer component={Paper} variant='outlined'>
                    <Table size='small'>
                        <TableHead>
                            <TableRow>
                                <TableCell>Zona</TableCell>
                                <TableCell>Sensor</TableCell>
                                <TableCell>Nilai</TableCell>
                                <TableCell>Waktu</TableCell>
                                <TableCell sx={{ width: 40 }}>✓</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {openAlerts.map((a) => (
                                <AlertRow key={a.id} alert={a} onResolve={fetchAll} />
                            ))}
                        </TableBody>
                    </Table>
                </TableContainer>
            )}

            <Box sx={{ mt: 2 }}>
                <Typography variant='caption' color='text.disabled'>
                    Data diperbarui otomatis setiap 30 detik. Untuk integrasi sensor fisik, kirim data ke{' '}
                    <code>POST /api/v1/iot/readings</code> dengan header <code>X-Internal-Key: ob-iot-internal-2026</code>. Simulator:{' '}
                    <code>node services/go-iot/simulator.js --scenario dry</code>
                </Typography>
            </Box>
        </MainCard>
    )
}
