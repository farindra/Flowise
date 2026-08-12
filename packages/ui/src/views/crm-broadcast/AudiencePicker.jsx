import PropTypes from 'prop-types'
import { useEffect, useState, useCallback, useRef } from 'react'
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Checkbox,
    Chip,
    CircularProgress,
    FormControl,
    FormControlLabel,
    InputLabel,
    ListItemText,
    MenuItem,
    OutlinedInput,
    Select,
    Slider,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableRow,
    Typography
} from '@mui/material'
import { IconChevronDown, IconDownload, IconUpload } from '@tabler/icons-react'
import broadcastApi from '@/api/crmbroadcast'

const TIERS = ['normal', 'vip', 'blacklist']

const SKIP_LABELS = {
    blacklist: 'blacklist',
    optout: 'berhenti langganan',
    cooldown: 'cooldown',
    invalid: 'nomor tidak valid',
    bot_number: 'nomor bot',
    dup: 'duplikat'
}

function formatDuration(seconds) {
    if (!seconds || seconds < 60) return '<1 menit'
    const h = Math.floor(seconds / 3600)
    const m = Math.round((seconds % 3600) / 60)
    if (h === 0) return `${m} menit`
    return `${h} jam ${m} menit`
}

const AudiencePicker = ({ value, onChange, includeBlacklist, onIncludeBlacklistChange, allPhones, onAllPhonesChange }) => {
    const [wilayahOpts, setWilayahOpts] = useState([])
    const [chatSources, setChatSources] = useState([])
    const [preview, setPreview] = useState(null)
    const [previewing, setPreviewing] = useState(false)
    const [error, setError] = useState('')
    const [uploadInfo, setUploadInfo] = useState(null)
    const fileRef = useRef(null)
    const debounceRef = useRef(null)

    useEffect(() => {
        broadcastApi
            .getWilayah()
            .then((r) => setWilayahOpts(r.data || []))
            .catch(() => setWilayahOpts([]))
        broadcastApi
            .getChatSources()
            .then((r) => setChatSources(r.data || []))
            .catch(() => setChatSources([]))
    }, [])

    const runPreview = useCallback(async () => {
        setPreviewing(true)
        setError('')
        try {
            const { data } = await broadcastApi.previewAudience({
                audience: value,
                include_blacklist: includeBlacklist,
                all_phones: allPhones
            })
            setPreview(data)
        } catch (e) {
            setError(e.response?.data?.error || e.message)
            setPreview(null)
        } finally {
            setPreviewing(false)
        }
    }, [value, includeBlacklist, allPhones])

    // Debounced so dragging the day slider doesn't fire a query per pixel.
    useEffect(() => {
        if (debounceRef.current) clearTimeout(debounceRef.current)
        debounceRef.current = setTimeout(runPreview, 500)
        return () => {
            if (debounceRef.current) clearTimeout(debounceRef.current)
        }
    }, [runPreview])

    const patch = (partial) => onChange({ ...value, ...partial })

    const toggleCustomers = (enabled) => patch({ customers: enabled ? { tiers: [], wilayahs: [] } : undefined })
    const toggleChat = (enabled) => patch({ chat: enabled ? { chatflow_ids: [], days: 90, include_passthrough: false } : undefined })

    const handleFile = async (e) => {
        const file = e.target.files?.[0]
        e.target.value = ''
        if (!file) return
        setError('')
        try {
            const fd = new FormData()
            fd.append('file', file)
            const { data } = await broadcastApi.uploadAudienceFile(fd)
            patch({ upload: data.rows || [] })
            setUploadInfo({ count: data.count, variables: data.variables || [], filename: file.name })
        } catch (err) {
            setError(err.response?.data?.error || err.message)
        }
    }

    const downloadTemplate = async () => {
        try {
            const res = await broadcastApi.downloadTemplate()
            const url = URL.createObjectURL(res.data)
            const a = document.createElement('a')
            a.href = url
            a.download = 'template-broadcast.xlsx'
            document.body.appendChild(a)
            a.click()
            a.remove()
            URL.revokeObjectURL(url)
        } catch (err) {
            setError(err.response?.data?.error || err.message)
        }
    }

    const chatflowOpts = chatSources.reduce((acc, s) => {
        if (!acc.find((x) => x.chatflow_id === s.chatflow_id)) {
            acc.push({ chatflow_id: s.chatflow_id, names: [s.name] })
        } else {
            acc.find((x) => x.chatflow_id === s.chatflow_id).names.push(s.name)
        }
        return acc
    }, [])

    const skipEntries = Object.entries(preview?.skipped || {}).filter(([, n]) => n > 0)

    return (
        <Stack spacing={2}>
            {error && <Alert severity='error'>{error}</Alert>}

            {/* Sumber 1 — Customers */}
            <Accordion defaultExpanded>
                <AccordionSummary
                    expandIcon={<IconChevronDown size={18} />}
                    sx={{ '& .MuiAccordionSummary-expandIconWrapper': { color: 'text.secondary' } }}
                >
                    <FormControlLabel
                        onClick={(e) => e.stopPropagation()}
                        control={<Checkbox checked={!!value.customers} onChange={(e) => toggleCustomers(e.target.checked)} />}
                        label='Customer di CRM'
                    />
                </AccordionSummary>
                <AccordionDetails>
                    {value.customers && (
                        <Stack spacing={2}>
                            <Stack direction='row' spacing={2}>
                                <FormControl size='small' sx={{ flex: 1 }}>
                                    <InputLabel>Tier</InputLabel>
                                    <Select
                                        multiple
                                        value={value.customers.tiers || []}
                                        onChange={(e) => patch({ customers: { ...value.customers, tiers: e.target.value } })}
                                        input={<OutlinedInput label='Tier' />}
                                        renderValue={(sel) => (sel.length ? sel.join(', ') : 'Semua tier')}
                                    >
                                        {TIERS.map((t) => (
                                            <MenuItem key={t} value={t}>
                                                <Checkbox checked={(value.customers.tiers || []).indexOf(t) > -1} />
                                                <ListItemText primary={t} />
                                            </MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                                <FormControl size='small' sx={{ flex: 1 }}>
                                    <InputLabel>Wilayah</InputLabel>
                                    <Select
                                        multiple
                                        value={value.customers.wilayahs || []}
                                        onChange={(e) => patch({ customers: { ...value.customers, wilayahs: e.target.value } })}
                                        input={<OutlinedInput label='Wilayah' />}
                                        renderValue={(sel) => (sel.length ? sel.join(', ') : 'Semua wilayah')}
                                    >
                                        {wilayahOpts.map((w) => (
                                            <MenuItem key={w.value} value={w.value}>
                                                <Checkbox checked={(value.customers.wilayahs || []).indexOf(w.value) > -1} />
                                                <ListItemText primary={`${w.label} (${w.count})`} />
                                            </MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                            </Stack>
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        color='warning'
                                        checked={includeBlacklist}
                                        onChange={(e) => onIncludeBlacklistChange(e.target.checked)}
                                    />
                                }
                                label={
                                    <Box>
                                        <Typography variant='body2'>Sertakan tier Blacklist</Typography>
                                        <Typography variant='caption' color='text.secondary'>
                                            Sebagian besar customer bertier blacklist — tanpa ini mereka tidak dikirimi
                                        </Typography>
                                    </Box>
                                }
                            />
                            <FormControlLabel
                                control={<Checkbox checked={allPhones} onChange={(e) => onAllPhonesChange(e.target.checked)} />}
                                label={
                                    <Box>
                                        <Typography variant='body2'>Kirim ke semua nomor customer</Typography>
                                        <Typography variant='caption' color='text.secondary'>
                                            Default hanya nomor pertama — nomor kedua biasanya line lain orang yang sama
                                        </Typography>
                                    </Box>
                                }
                            />
                        </Stack>
                    )}
                </AccordionDetails>
            </Accordion>

            {/* Sumber 2 — Upload */}
            <Accordion>
                <AccordionSummary
                    expandIcon={<IconChevronDown size={18} />}
                    sx={{ '& .MuiAccordionSummary-expandIconWrapper': { color: 'text.secondary' } }}
                >
                    <Typography sx={{ alignSelf: 'center' }}>Upload Excel/CSV {uploadInfo ? `(${uploadInfo.count} baris)` : ''}</Typography>
                </AccordionSummary>
                <AccordionDetails>
                    <Stack spacing={2}>
                        <Stack direction='row' spacing={1}>
                            <Chip
                                size='small'
                                variant='outlined'
                                label='Unduh Template'
                                clickable
                                icon={<IconDownload size={14} />}
                                onClick={downloadTemplate}
                            />
                            <Chip
                                size='small'
                                color='primary'
                                label='Pilih File'
                                clickable
                                icon={<IconUpload size={14} />}
                                onClick={() => fileRef.current?.click()}
                            />
                            <input ref={fileRef} type='file' accept='.xlsx,.csv' hidden onChange={handleFile} />
                        </Stack>
                        {uploadInfo && (
                            <Alert severity='success'>
                                {uploadInfo.filename}: {uploadInfo.count} baris.
                                {uploadInfo.variables.length > 0 && (
                                    <>
                                        {' '}
                                        Variabel tambahan:{' '}
                                        {uploadInfo.variables.map((v) => (
                                            <code key={v}>{`{{${v}}} `}</code>
                                        ))}
                                    </>
                                )}
                            </Alert>
                        )}
                        <Typography variant='caption' color='text.secondary'>
                            Kolom wajib: Nama, No WA, Wilayah. Kolom lain otomatis jadi variabel template.
                        </Typography>
                    </Stack>
                </AccordionDetails>
            </Accordion>

            {/* Sumber 3 — Riwayat chat */}
            <Accordion>
                <AccordionSummary
                    expandIcon={<IconChevronDown size={18} />}
                    sx={{ '& .MuiAccordionSummary-expandIconWrapper': { color: 'text.secondary' } }}
                >
                    <FormControlLabel
                        onClick={(e) => e.stopPropagation()}
                        control={<Checkbox checked={!!value.chat} onChange={(e) => toggleChat(e.target.checked)} />}
                        label='Riwayat Chat Bot'
                    />
                </AccordionSummary>
                <AccordionDetails>
                    {value.chat && (
                        <Stack spacing={2}>
                            <FormControl size='small' fullWidth>
                                <InputLabel>Bot</InputLabel>
                                <Select
                                    multiple
                                    value={value.chat.chatflow_ids || []}
                                    onChange={(e) => patch({ chat: { ...value.chat, chatflow_ids: e.target.value } })}
                                    input={<OutlinedInput label='Bot' />}
                                    renderValue={(sel) =>
                                        sel.map((id) => chatflowOpts.find((c) => c.chatflow_id === id)?.names.join(' / ') || id).join(', ')
                                    }
                                >
                                    {chatflowOpts.map((c) => (
                                        <MenuItem key={c.chatflow_id} value={c.chatflow_id}>
                                            <Checkbox checked={(value.chat.chatflow_ids || []).indexOf(c.chatflow_id) > -1} />
                                            <ListItemText primary={c.names.join(' / ')} />
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            <Box>
                                <Typography variant='caption' color='text.secondary'>
                                    Chat dalam {value.chat.days} hari terakhir
                                </Typography>
                                <Slider
                                    size='small'
                                    min={7}
                                    max={365}
                                    step={7}
                                    value={value.chat.days}
                                    onChange={(e, v) => patch({ chat: { ...value.chat, days: v } })}
                                    valueLabelDisplay='auto'
                                />
                            </Box>
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        checked={!!value.chat.include_passthrough}
                                        onChange={(e) => patch({ chat: { ...value.chat, include_passthrough: e.target.checked } })}
                                    />
                                }
                                label={
                                    <Box>
                                        <Typography variant='body2'>Sertakan chatId yang sudah berbentuk nomor</Typography>
                                        <Typography variant='caption' color='text.secondary'>
                                            Tidak disarankan — bisa berisi ID dari platform lain (mis. Telegram)
                                        </Typography>
                                    </Box>
                                }
                            />
                        </Stack>
                    )}
                </AccordionDetails>
            </Accordion>

            {/* Preview live.
                Text inside an Alert must inherit the Alert's own colour: the
                app theme never sets palette.mode, so MUI paints Alert
                backgrounds as if in light mode while Typography would default
                to text.primary (white in dark mode) and vanish. */}
            <Alert severity={preview?.total > 0 ? 'info' : 'warning'} icon={previewing ? <CircularProgress size={18} /> : undefined}>
                <Typography variant='body2' fontWeight={600} color='inherit'>
                    {previewing ? 'Menghitung...' : `${preview?.total ?? 0} penerima`}
                    {skipEntries.length > 0 && (
                        <>
                            {' · '}
                            {skipEntries.reduce((a, [, n]) => a + n, 0)} dilewati (
                            {skipEntries.map(([k, n]) => `${SKIP_LABELS[k] || k} ${n}`).join(', ')})
                        </>
                    )}
                    {preview?.unresolved_count > 0 && ` · ${preview.unresolved_count} chat tidak terpetakan`}
                </Typography>
                {preview?.total > 0 && (
                    <Typography variant='caption' color='inherit' sx={{ opacity: 0.85 }}>
                        Estimasi durasi kirim: {formatDuration(preview.estimate_seconds)}
                    </Typography>
                )}
                {(preview?.warnings || []).map((w, i) => (
                    <Typography key={i} variant='caption' display='block' color='inherit' sx={{ opacity: 0.85 }}>
                        • {w}
                    </Typography>
                ))}
            </Alert>

            {preview?.sample?.length > 0 && (
                <Table size='small'>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama</TableCell>
                            <TableCell>No WA</TableCell>
                            <TableCell>Wilayah</TableCell>
                            <TableCell>Tier</TableCell>
                            <TableCell>Sumber</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {preview.sample.map((s) => (
                            <TableRow key={s.phone}>
                                <TableCell>{s.name || <span style={{ opacity: 0.6 }}>—</span>}</TableCell>
                                <TableCell>{s.phone}</TableCell>
                                <TableCell>{s.wilayah || '—'}</TableCell>
                                <TableCell>{s.tier || '—'}</TableCell>
                                <TableCell>{s.source}</TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            )}
        </Stack>
    )
}

AudiencePicker.propTypes = {
    value: PropTypes.object.isRequired,
    onChange: PropTypes.func.isRequired,
    includeBlacklist: PropTypes.bool,
    onIncludeBlacklistChange: PropTypes.func.isRequired,
    allPhones: PropTypes.bool,
    onAllPhonesChange: PropTypes.func.isRequired
}

export default AudiencePicker
