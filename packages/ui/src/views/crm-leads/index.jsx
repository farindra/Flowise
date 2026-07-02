import PropTypes from 'prop-types'
import { useEffect, useState, useCallback } from 'react'
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Divider,
    Drawer,
    FormControl,
    Grid,
    IconButton,
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
import {
    IconRefresh,
    IconPhone,
    IconUser,
    IconChevronRight,
    IconBrandWhatsapp,
    IconMail,
    IconX,
    IconCalendar,
    IconTag,
    IconAlertTriangle,
    IconCurrencyDollar,
    IconShoppingBag,
    IconMessage,
    IconUserCheck
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/crm'

const STAGES = [
    { value: '', label: 'Semua Stage' },
    { value: 'new', label: '🆕 New' },
    { value: 'contacted', label: '📞 Contacted' },
    { value: 'qualified', label: '✅ Qualified' },
    { value: 'proposal', label: '📋 Proposal' },
    { value: 'negotiation', label: '🤝 Negotiation' },
    { value: 'won', label: '🎉 Won' },
    { value: 'lost', label: '❌ Lost' }
]

const STAGE_COLOR = {
    new: 'default',
    contacted: 'info',
    qualified: 'primary',
    proposal: 'warning',
    negotiation: 'secondary',
    won: 'success',
    lost: 'error'
}

const STAGE_EMOJI = { new: '🆕', contacted: '📞', qualified: '✅', proposal: '📋', negotiation: '🤝', won: '🎉', lost: '❌' }
const URGENCY_COLOR = { high: 'error', medium: 'warning', low: 'default' }
const URGENCY_OPTIONS = ['high', 'medium', 'low']
const BUDGET_OPTIONS = ['< 50 jt', '50–100 jt', '100–250 jt', '250–500 jt', '> 500 jt']

function StatsBar({ stats }) {
    if (!stats) return null
    const items = [
        { label: 'Total Lead', value: stats.total_leads ?? '—' },
        { label: 'New', value: stats.by_stage?.new ?? 0, color: '#64748b' },
        { label: 'Contacted', value: stats.by_stage?.contacted ?? 0, color: '#0ea5e9' },
        { label: 'Qualified', value: stats.by_stage?.qualified ?? 0, color: '#6366f1' },
        { label: 'Won', value: stats.by_stage?.won ?? 0, color: '#22c55e' },
        { label: 'Lost', value: stats.by_stage?.lost ?? 0, color: '#ef4444' }
    ]
    return (
        <Grid container spacing={1.5} sx={{ mb: 2 }}>
            {items.map((it) => (
                <Grid item key={it.label}>
                    <Paper variant='outlined' sx={{ px: 2, py: 1, minWidth: 90, textAlign: 'center' }}>
                        <Typography variant='h5' sx={{ color: it.color, fontWeight: 700 }}>
                            {it.value}
                        </Typography>
                        <Typography variant='caption' color='text.secondary'>
                            {it.label}
                        </Typography>
                    </Paper>
                </Grid>
            ))}
        </Grid>
    )
}
StatsBar.propTypes = { stats: PropTypes.object }

function InfoRow({ icon, label, value, mono }) {
    if (!value) return null
    return (
        <Stack direction='row' spacing={1.5} alignItems='flex-start' sx={{ py: 0.75 }}>
            <Box sx={{ color: 'text.secondary', mt: 0.2, flexShrink: 0 }}>{icon}</Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography variant='caption' color='text.secondary' display='block'>
                    {label}
                </Typography>
                <Typography variant='body2' sx={mono ? { fontFamily: 'monospace' } : {}} noWrap={false}>
                    {value}
                </Typography>
            </Box>
        </Stack>
    )
}
InfoRow.propTypes = { icon: PropTypes.node, label: PropTypes.string, value: PropTypes.any, mono: PropTypes.bool }

function LeadDrawer({ lead, onClose, onSaved, salesmen }) {
    const [form, setForm] = useState({
        stage: '',
        urgency: '',
        score: 50,
        budget_range: '',
        interest: '',
        notes: '',
        assigned_to: '',
        follow_up_at: ''
    })
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState(null)

    useEffect(() => {
        if (!lead) return
        setForm({
            stage: lead.stage || 'new',
            urgency: lead.urgency || 'medium',
            score: lead.score ?? 50,
            budget_range: lead.budget_range || '',
            interest: lead.interest || '',
            notes: lead.notes || '',
            assigned_to: lead.assigned_to || '',
            follow_up_at: lead.follow_up_at ? new Date(lead.follow_up_at).toISOString().slice(0, 16) : ''
        })
        setError(null)
    }, [lead])

    const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

    const save = async () => {
        setSaving(true)
        setError(null)
        try {
            const payload = {
                stage: form.stage,
                urgency: form.urgency,
                score: Number(form.score),
                budget_range: form.budget_range,
                interest: form.interest,
                notes: form.notes,
                assigned_to: form.assigned_to,
                follow_up_at: form.follow_up_at ? new Date(form.follow_up_at).toISOString() : null
            }
            const res = await fetch(`${API}/leads/${lead.id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
            if (!res.ok) throw new Error('Gagal simpan')
            onSaved()
            onClose()
        } catch (e) {
            setError(e.message)
        } finally {
            setSaving(false)
        }
    }

    const fmtDate = (s) => (s ? new Date(s).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' }) : null)
    const waLink = lead?.phone
        ? `https://wa.me/${lead.phone.replace(/\D/g, '')}?text=${encodeURIComponent(
              "Assalamu'alaikum " + (lead?.name || '') + ', kami dari Al Azhar Memorial Garden'
          )}`
        : null

    return (
        <Drawer
            anchor='right'
            open={Boolean(lead)}
            onClose={onClose}
            PaperProps={{ sx: { width: { xs: '100vw', sm: 480 }, display: 'flex', flexDirection: 'column' } }}
        >
            {lead && (
                <>
                    {/* Header */}
                    <Box sx={{ px: 3, py: 2, bgcolor: 'primary.main', color: '#fff', flexShrink: 0 }}>
                        <Stack direction='row' justifyContent='space-between' alignItems='flex-start'>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography variant='h6' fontWeight={700} noWrap>
                                    {lead.name || <em style={{ opacity: 0.7 }}>Tanpa nama</em>}
                                </Typography>
                                <Stack direction='row' spacing={1} alignItems='center' sx={{ mt: 0.5 }}>
                                    <Typography variant='body2' sx={{ opacity: 0.85, fontFamily: 'monospace' }}>
                                        {lead.phone}
                                    </Typography>
                                    {waLink && (
                                        <Tooltip title='Buka WhatsApp'>
                                            <IconButton
                                                size='small'
                                                component='a'
                                                href={waLink}
                                                target='_blank'
                                                sx={{ color: '#fff', p: 0.3 }}
                                            >
                                                <IconBrandWhatsapp size={16} />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </Box>
                            <Stack direction='row' spacing={0.5} alignItems='center'>
                                <Chip
                                    size='small'
                                    label={`${STAGE_EMOJI[lead.stage] || ''} ${lead.stage}`}
                                    sx={{ bgcolor: 'rgba(255,255,255,.2)', color: '#fff', fontWeight: 600 }}
                                />
                                <IconButton size='small' onClick={onClose} sx={{ color: '#fff' }}>
                                    <IconX size={18} />
                                </IconButton>
                            </Stack>
                        </Stack>
                    </Box>

                    <Box sx={{ flex: 1, overflowY: 'auto', px: 3, py: 2 }}>
                        {/* Info profile */}
                        <Typography variant='overline' color='text.secondary' sx={{ letterSpacing: 1.5, fontWeight: 600 }}>
                            Informasi Lead
                        </Typography>
                        <Paper variant='outlined' sx={{ px: 2, py: 1, mt: 0.5, mb: 2 }}>
                            <InfoRow icon={<IconMail size={15} />} label='Email' value={lead.email} />
                            <InfoRow icon={<IconTag size={15} />} label='Source' value={lead.source} />
                            {lead.campaign_id && <InfoRow icon={<IconTag size={15} />} label='Campaign ID' value={lead.campaign_id} mono />}
                            <InfoRow icon={<IconShoppingBag size={15} />} label='Minat Produk' value={lead.interest} />
                            <InfoRow icon={<IconCurrencyDollar size={15} />} label='Budget' value={lead.budget_range} />
                            <InfoRow icon={<IconCalendar size={15} />} label='Masuk' value={fmtDate(lead.created_at)} />
                            <InfoRow icon={<IconCalendar size={15} />} label='Update' value={fmtDate(lead.updated_at)} />
                            {lead.last_contact_at && (
                                <InfoRow icon={<IconPhone size={15} />} label='Terakhir Kontak' value={fmtDate(lead.last_contact_at)} />
                            )}
                            {lead.notes && (
                                <Stack direction='row' spacing={1.5} alignItems='flex-start' sx={{ py: 0.75 }}>
                                    <Box sx={{ color: 'text.secondary', mt: 0.2, flexShrink: 0 }}>
                                        <IconMessage size={15} />
                                    </Box>
                                    <Box>
                                        <Typography variant='caption' color='text.secondary' display='block'>
                                            Catatan awal
                                        </Typography>
                                        <Typography variant='body2' sx={{ whiteSpace: 'pre-wrap' }}>
                                            {lead.notes}
                                        </Typography>
                                    </Box>
                                </Stack>
                            )}
                        </Paper>

                        <Divider sx={{ mb: 2 }} />

                        {/* Form Edit */}
                        <Typography variant='overline' color='text.secondary' sx={{ letterSpacing: 1.5, fontWeight: 600 }}>
                            Update Lead
                        </Typography>

                        {error && (
                            <Alert severity='error' sx={{ mt: 1, mb: 1 }}>
                                {error}
                            </Alert>
                        )}

                        <Stack spacing={2} sx={{ mt: 1 }}>
                            <FormControl size='small' fullWidth>
                                <InputLabel>Stage</InputLabel>
                                <Select value={form.stage} label='Stage' onChange={set('stage')}>
                                    {STAGES.filter((s) => s.value).map((s) => (
                                        <MenuItem key={s.value} value={s.value}>
                                            {s.label}
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>

                            <Stack direction='row' spacing={1.5}>
                                <FormControl size='small' fullWidth>
                                    <InputLabel>Urgensi</InputLabel>
                                    <Select value={form.urgency} label='Urgensi' onChange={set('urgency')}>
                                        {URGENCY_OPTIONS.map((u) => (
                                            <MenuItem key={u} value={u}>
                                                <Chip size='small' label={u} color={URGENCY_COLOR[u]} sx={{ fontSize: 11 }} />
                                            </MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                                <TextField
                                    size='small'
                                    label='Score'
                                    type='number'
                                    value={form.score}
                                    onChange={set('score')}
                                    inputProps={{ min: 0, max: 100 }}
                                    sx={{ width: 90, flexShrink: 0 }}
                                />
                            </Stack>

                            <FormControl size='small' fullWidth>
                                <InputLabel>Assigned Salesman</InputLabel>
                                <Select value={form.assigned_to} label='Assigned Salesman' onChange={set('assigned_to')}>
                                    <MenuItem value=''>
                                        <em>— tidak di-assign —</em>
                                    </MenuItem>
                                    {salesmen.map((s) => (
                                        <MenuItem key={s.id} value={s.name}>
                                            {s.name}
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>

                            <FormControl size='small' fullWidth>
                                <InputLabel>Budget Range</InputLabel>
                                <Select value={form.budget_range} label='Budget Range' onChange={set('budget_range')}>
                                    <MenuItem value=''>
                                        <em>— belum diketahui —</em>
                                    </MenuItem>
                                    {BUDGET_OPTIONS.map((b) => (
                                        <MenuItem key={b} value={b}>
                                            {b}
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>

                            <TextField
                                size='small'
                                fullWidth
                                label='Minat / Produk'
                                value={form.interest}
                                onChange={set('interest')}
                                placeholder='Kavling standar, keluarga, ...'
                            />

                            <TextField
                                size='small'
                                fullWidth
                                label='Follow Up Terjadwal'
                                type='datetime-local'
                                value={form.follow_up_at}
                                onChange={set('follow_up_at')}
                                InputLabelProps={{ shrink: true }}
                            />

                            <TextField
                                size='small'
                                fullWidth
                                label='Catatan / Update'
                                multiline
                                rows={4}
                                value={form.notes}
                                onChange={set('notes')}
                                placeholder='Catatan perkembangan, hasil follow up, preferensi, ...'
                            />
                        </Stack>
                    </Box>

                    {/* Footer actions */}
                    <Box sx={{ px: 3, py: 2, borderTop: '1px solid', borderColor: 'divider', flexShrink: 0 }}>
                        <Stack direction='row' spacing={1.5}>
                            {waLink && (
                                <Button
                                    variant='outlined'
                                    color='success'
                                    size='small'
                                    startIcon={<IconBrandWhatsapp size={16} />}
                                    component='a'
                                    href={waLink}
                                    target='_blank'
                                    sx={{ flex: 1 }}
                                >
                                    WhatsApp
                                </Button>
                            )}
                            <Button variant='outlined' size='small' onClick={onClose} sx={{ flexShrink: 0 }}>
                                Tutup
                            </Button>
                            <Button
                                variant='contained'
                                size='small'
                                onClick={save}
                                disabled={saving}
                                startIcon={saving ? <CircularProgress size={14} color='inherit' /> : <IconUserCheck size={16} />}
                                sx={{ flex: 1 }}
                            >
                                {saving ? 'Menyimpan...' : 'Simpan'}
                            </Button>
                        </Stack>
                    </Box>
                </>
            )}
        </Drawer>
    )
}

LeadDrawer.propTypes = {
    lead: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired,
    salesmen: PropTypes.array
}

export default function CRMLeads() {
    const [leads, setLeads] = useState([])
    const [stats, setStats] = useState(null)
    const [salesmen, setSalesmen] = useState([])
    const [stageFilter, setStageFilter] = useState('')
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)
    const [selectedLead, setSelectedLead] = useState(null)

    const fetchAll = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const qs = stageFilter ? `?stage=${stageFilter}` : ''
            const [leadsRes, statsRes, salesmenRes] = await Promise.all([
                fetch(`${API}/leads${qs}`),
                fetch(`${API}/stats?period=month`),
                fetch(`${API}/salesmen`)
            ])
            const [leadsData, statsData, salesmenData] = await Promise.all([leadsRes.json(), statsRes.json(), salesmenRes.json()])
            setLeads(Array.isArray(leadsData) ? leadsData : [])
            setStats(statsData)
            setSalesmen(Array.isArray(salesmenData) ? salesmenData : [])
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [stageFilter])

    useEffect(() => {
        fetchAll()
    }, [fetchAll])

    const fmtDate = (s) => (s ? new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'short', year: '2-digit' }) : '—')

    return (
        <MainCard
            title='CRM — Pipeline Leads'
            secondary={
                <IconButton
                    size='small'
                    onClick={fetchAll}
                    disabled={loading}
                    sx={{ color: 'text.primary', bgcolor: 'action.hover', '&:hover': { bgcolor: 'action.selected' } }}
                >
                    {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                </IconButton>
            }
        >
            <StatsBar stats={stats} />

            <Stack direction='row' spacing={1.5} sx={{ mb: 2 }} alignItems='center'>
                <FormControl size='small' sx={{ minWidth: 180 }}>
                    <InputLabel>Filter Stage</InputLabel>
                    <Select value={stageFilter} label='Filter Stage' onChange={(e) => setStageFilter(e.target.value)}>
                        {STAGES.map((s) => (
                            <MenuItem key={s.value} value={s.value}>
                                {s.label}
                            </MenuItem>
                        ))}
                    </Select>
                </FormControl>
                <Typography variant='caption' color='text.secondary'>
                    {leads.length} lead{leads.length !== 1 ? 's' : ''}
                </Typography>
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
                            <TableCell>Telepon</TableCell>
                            <TableCell>Stage</TableCell>
                            <TableCell>Urgensi</TableCell>
                            <TableCell>Source</TableCell>
                            <TableCell>Assigned</TableCell>
                            <TableCell>Masuk</TableCell>
                            <TableCell sx={{ width: 36 }} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {leads.length === 0 && !loading && (
                            <TableRow>
                                <TableCell colSpan={8} align='center' sx={{ py: 4, color: 'text.disabled' }}>
                                    Belum ada leads
                                </TableCell>
                            </TableRow>
                        )}
                        {leads.map((lead) => (
                            <TableRow
                                key={lead.id}
                                hover
                                sx={{ cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' } }}
                                onClick={() => setSelectedLead(lead)}
                            >
                                <TableCell>
                                    <Stack direction='row' spacing={0.5} alignItems='center'>
                                        <IconUser size={14} />
                                        <Typography variant='body2'>{lead.name || <em style={{ color: '#888' }}>—</em>}</Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Stack direction='row' spacing={0.5} alignItems='center'>
                                        <IconPhone size={14} />
                                        <Typography variant='body2' sx={{ fontFamily: 'monospace' }}>
                                            {lead.phone}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        size='small'
                                        label={`${STAGE_EMOJI[lead.stage] || ''} ${lead.stage}`}
                                        color={STAGE_COLOR[lead.stage] || 'default'}
                                        sx={{ fontSize: 11 }}
                                    />
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        size='small'
                                        label={lead.urgency}
                                        color={URGENCY_COLOR[lead.urgency] || 'default'}
                                        sx={{ fontSize: 11 }}
                                    />
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {lead.source}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Stack direction='row' spacing={0.5} alignItems='center'>
                                        {lead.assigned_to ? (
                                            <>
                                                <IconAlertTriangle size={12} style={{ opacity: 0 }} />
                                                <Typography variant='caption'>{lead.assigned_to}</Typography>
                                            </>
                                        ) : (
                                            <Typography variant='caption' color='text.disabled'>
                                                —
                                            </Typography>
                                        )}
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {fmtDate(lead.created_at)}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <IconChevronRight size={16} color='#888' />
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </TableContainer>

            <LeadDrawer lead={selectedLead} onClose={() => setSelectedLead(null)} onSaved={fetchAll} salesmen={salesmen} />
        </MainCard>
    )
}
