import { useState, useEffect } from 'react'
import { format } from 'date-fns'
import { Lock, Copy, Check } from 'lucide-react'
import { useAuthStore } from '@/store'
import { ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import {
  useOrg, useUpdateOrgSettings, useOrgMembers, useRemoveMember,
  useInvites, useCreateInvite, useRevokeInvite,
  useAlertRules, useCreateAlertRule, useUpdateAlertRule, useDeleteAlertRule,
  useInitiateDeletion, useCancelDeletion,
} from '@/hooks/use-org'
import type { AlertRule, OrgMember, Invite } from '@/types/api'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

type TabKey = 'general' | 'members' | 'invites' | 'alerts'

function MinimalTabs({ active, onChange }: { active: TabKey; onChange: (t: TabKey) => void }) {
  const { t } = useI18n()
  const tabs: { key: TabKey; label: string }[] = [
    { key: 'general', label: t.settings_tab_general }, { key: 'members', label: t.settings_tab_members },
    { key: 'invites', label: t.settings_tab_invites }, { key: 'alerts', label: t.settings_tab_alerts },
  ]
  return (
    <div className="flex items-center gap-0.5 bg-zinc-900 border border-zinc-800 rounded-md p-1 w-fit overflow-x-auto">
      {tabs.map((tab) => (
        <button key={tab.key} onClick={() => onChange(tab.key)}
          className={`px-3 py-1 text-sm rounded transition-colors duration-150 ${active === tab.key ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'}`}
        >{tab.label}</button>
      ))}
    </div>
  )
}

function GeneralTab({ orgID }: { orgID: string }) {
  const { t, tt } = useI18n()
  const { data: org } = useOrg(orgID)
  const updateSettings = useUpdateOrgSettings(orgID)
  const initiateDeletion = useInitiateDeletion(orgID)
  const cancelDeletion = useCancelDeletion(orgID)
  const [name, setName] = useState(''); const [locale, setLocale] = useState('en'); const [sessionTimeout, setSessionTimeout] = useState(60)
  const [isDirty, setIsDirty] = useState(false); const [saveError, setSaveError] = useState('')
  const [showDeleteDialog, setShowDeleteDialog] = useState(false); const [showCancelDeleteDialog, setShowCancelDeleteDialog] = useState(false)

  useEffect(() => { if (org) { setName(org.name); setLocale(org.locale || 'en'); setSessionTimeout(org.session_timeout_seconds); setIsDirty(false) } }, [org])
  function markDirty() { setIsDirty(true) }
  async function handleSave() {
    setSaveError('')
    try {
      await updateSettings.mutateAsync({ name: name.trim(), locale, session_timeout_seconds: sessionTimeout })
      setIsDirty(false)
      // Sync UI language to match org locale (D-07)
      if (locale === 'en' || locale === 'ru') {
        useI18n.getState().setLang(locale)
      }
    } catch { setSaveError(t.settings_save_error) }
  }
  async function handleDeleteOrg() { await initiateDeletion.mutateAsync(); setShowDeleteDialog(false) }
  async function handleCancelDelete() { await cancelDeletion.mutateAsync(); setShowCancelDeleteDialog(false) }

  return (
    <div className="space-y-6 max-w-lg">
      <div className="space-y-4">
        <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_org_name}</label><input value={name} onChange={(e) => { setName(e.target.value); markDirty() }} className={inputClass} /></div>
        <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_locale}</label>
          <Select value={locale} onValueChange={(v) => { setLocale(v); markDirty() }}>
            <SelectTrigger className="bg-zinc-900 border-zinc-800 text-zinc-100"><SelectValue /></SelectTrigger>
            <SelectContent className="bg-zinc-900 border-zinc-800">
              <SelectItem value="en" className="text-zinc-100">{t.settings_locale_en}</SelectItem>
              <SelectItem value="ru" className="text-zinc-100">{t.settings_locale_ru}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_session_timeout}</label>
          <div className="flex items-center gap-2"><input type="number" value={sessionTimeout} onChange={(e) => { setSessionTimeout(Number(e.target.value)); markDirty() }} className={cn(inputClass, 'w-32')} /><span className="text-sm text-zinc-500">{t.settings_seconds}</span></div>
        </div>
        {saveError && <p className="text-sm text-red-400">{saveError}</p>}
        <button onClick={handleSave} disabled={!isDirty || updateSettings.isPending} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors disabled:opacity-50 btn-press">{updateSettings.isPending ? t.settings_saving : t.settings_save}</button>
      </div>

      <div className="h-px bg-zinc-800 my-8" />

      <div className="space-y-4">
        <h3 className="text-sm font-medium text-red-400">{t.settings_danger}</h3>
        {org?.deletion_scheduled_at?.Valid ? (
          <div className="rounded-md bg-red-500/10 border border-red-500/20 p-4 space-y-3">
            <p className="text-sm text-red-400">{tt('settings_deletion_scheduled', { date: format(new Date(org.deletion_scheduled_at.Time), 'PPP') })}</p>
            <button onClick={() => setShowCancelDeleteDialog(true)} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors">{t.settings_cancel_deletion}</button>
          </div>
        ) : (
          <button onClick={() => setShowDeleteDialog(true)} className="text-sm font-medium bg-red-500/10 text-red-400 px-4 py-2 rounded-md hover:bg-red-500/20 transition-colors">{t.settings_delete_org}</button>
        )}
      </div>

      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50"><DialogHeader><DialogTitle>{t.settings_delete_title}</DialogTitle></DialogHeader>
          <p className="text-sm text-zinc-400 py-2">{t.settings_delete_body}</p>
          <DialogFooter><Button variant="ghost" onClick={() => setShowDeleteDialog(false)} className="text-zinc-400">{t.settings_delete_dialog_cancel}</Button>
            <button onClick={handleDeleteOrg} disabled={initiateDeletion.isPending} className="text-sm font-medium bg-red-500/10 text-red-400 px-4 py-2 rounded-md hover:bg-red-500/20 disabled:opacity-50">{t.settings_delete_dialog_confirm}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={showCancelDeleteDialog} onOpenChange={setShowCancelDeleteDialog}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50"><DialogHeader><DialogTitle>{t.settings_cancel_delete_title}</DialogTitle></DialogHeader>
          <p className="text-sm text-zinc-400 py-2">{t.settings_cancel_delete_body}</p>
          <DialogFooter><Button variant="ghost" onClick={() => setShowCancelDeleteDialog(false)} className="text-zinc-400">{t.settings_keep_deleting}</Button>
            <button onClick={handleCancelDelete} disabled={cancelDeletion.isPending} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 disabled:opacity-50">{t.settings_cancel_delete_confirm}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MembersTab({ orgID }: { orgID: string }) {
  const { t, tt } = useI18n()
  const { data: org } = useOrg(orgID); const { data: members, isLoading } = useOrgMembers(orgID); const removeMember = useRemoveMember(orgID)
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null)
  if (org?.plan === 'free') return <div className="text-center py-16"><p className="text-base font-medium text-zinc-200 mb-2">{t.settings_members_free_title}</p><p className="text-sm text-zinc-500">{t.settings_members_free_body}</p></div>
  async function handleRemove(id: string) { await removeMember.mutateAsync(id); setConfirmRemove(null) }
  if (isLoading) return <p className="text-sm text-zinc-500">{t.settings_members_loading}</p>
  if (!members || members.length === 0) return <p className="text-sm text-zinc-500">{t.settings_members_empty}</p>
  return (
    <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-sm text-left">
          <thead><tr className="border-b border-zinc-800">
            <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_members_col_name}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_members_col_email}</th>
            <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_members_col_role}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden sm:table-cell">{t.settings_members_col_joined}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider"></th>
          </tr></thead>
          <tbody className="divide-y divide-zinc-800/40">
            {members.map((m: OrgMember) => (
              <tr key={m.user_id} className="table-row-hover">
                {confirmRemove === m.user_id ? (
                  <td colSpan={5} className="px-5 py-3.5"><span className="text-zinc-200 mr-4">{tt('settings_members_remove_inline', { name: m.user_name })}</span>
                    <button onClick={() => handleRemove(m.user_id)} className="text-sm text-red-400 mr-2">{t.settings_members_confirm}</button>
                    <button onClick={() => setConfirmRemove(null)} className="text-sm text-zinc-500">{t.settings_members_cancel}</button>
                  </td>
                ) : (<>
                  <td className="px-5 py-3.5 text-zinc-200 font-medium">{m.user_name}</td><td className="px-5 py-3.5 text-zinc-500">{m.email}</td>
                  <td className="px-5 py-3.5 text-zinc-500 capitalize">{m.role}</td><td className="px-5 py-3.5 text-zinc-600 hidden sm:table-cell">{m.created_at ? format(new Date(m.created_at), 'PP') : '\u2014'}</td>
                  <td className="px-5 py-3.5">{m.role !== 'owner' && <button onClick={() => setConfirmRemove(m.user_id)} className="text-sm text-red-400 hover:text-red-300">{t.settings_members_remove}</button>}</td>
                </>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function InvitesTab({ orgID }: { orgID: string }) {
  const { t } = useI18n()
  const { data: invites, isLoading } = useInvites(orgID); const createInvite = useCreateInvite(orgID); const revokeInvite = useRevokeInvite(orgID)
  const [showInviteDialog, setShowInviteDialog] = useState(false); const [inviteEmail, setInviteEmail] = useState(''); const [inviteRole, setInviteRole] = useState('member')
  const [inviteError, setInviteError] = useState(''); const [copiedInviteID, setCopiedInviteID] = useState<string | null>(null); const [confirmRevoke, setConfirmRevoke] = useState<string | null>(null)
  function resetInviteForm() { setInviteEmail(''); setInviteRole('member'); setInviteError('') }
  async function handleSendInvite() { if (!inviteEmail.trim()) { setInviteError(t.auth_field_required); return }; setInviteError(''); try { await createInvite.mutateAsync({ email: inviteEmail.trim(), role: inviteRole }); setShowInviteDialog(false); resetInviteForm() } catch { setInviteError(t.settings_invite_error) } }
  function handleCopyLink(invite: Invite) { if (!invite.invite_url) return; navigator.clipboard.writeText(invite.invite_url).catch(() => {}); setCopiedInviteID(invite.id); setTimeout(() => setCopiedInviteID(null), 2000) }
  async function handleRevoke(id: string) { await revokeInvite.mutateAsync(id); setConfirmRevoke(null) }

  return (
    <div>
      <div className="flex justify-end mb-6">
        <button onClick={() => { resetInviteForm(); setShowInviteDialog(true) }} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-3.5 py-1.5 rounded-md hover:bg-zinc-200 transition-colors">{t.settings_invite_btn}</button>
      </div>
      {isLoading ? <p className="text-sm text-zinc-500">{t.settings_invites_loading}</p> : !invites || invites.length === 0 ? (
        <div className="text-center py-16"><p className="text-base font-medium text-zinc-200 mb-2">{t.settings_invites_empty_title}</p><p className="text-sm text-zinc-500">{t.settings_invites_empty_body}</p></div>
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left"><thead><tr className="border-b border-zinc-800">
              <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_invites_col_email}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_invites_col_role}</th>
              <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden sm:table-cell">{t.settings_invites_col_expires}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider"></th>
            </tr></thead><tbody className="divide-y divide-zinc-800/40">
              {invites.map((inv: Invite) => (
                <tr key={inv.id} className="table-row-hover">
                  <td className="px-5 py-3.5 text-zinc-200">{inv.email}</td><td className="px-5 py-3.5 text-zinc-500 capitalize">{inv.role}</td>
                  <td className="px-5 py-3.5 text-zinc-600 hidden sm:table-cell">{inv.expires_at ? format(new Date(inv.expires_at), 'PP') : '\u2014'}</td>
                <td className="px-5 py-3.5">
                  <div className="flex items-center gap-2">
                    {inv.invite_url && <button onClick={() => handleCopyLink(inv)} className="text-zinc-500 hover:text-zinc-200 transition-colors">{copiedInviteID === inv.id ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}</button>}
                    {confirmRevoke === inv.id ? (<><button onClick={() => handleRevoke(inv.id)} className="text-sm text-red-400">{t.settings_invites_revoke_confirm}</button><button onClick={() => setConfirmRevoke(null)} className="text-sm text-zinc-500 ml-2">{t.settings_invites_revoke_cancel}</button></>) :
                      <button onClick={() => setConfirmRevoke(inv.id)} className="text-sm text-red-400 hover:text-red-300">{t.settings_invites_revoke}</button>}
                  </div>
                </td>
              </tr>
            ))}
            </tbody></table>
          </div>
        </div>
      )}
      <Dialog open={showInviteDialog} onOpenChange={(o) => { if (!o) { setShowInviteDialog(false); resetInviteForm() } }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50"><DialogHeader><DialogTitle>{t.settings_invite_title}</DialogTitle></DialogHeader>
          <div className="space-y-4 py-2">
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_invite_email}</label><input type="email" placeholder={t.settings_invite_email_placeholder} value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)} className={inputClass} /></div>
            <div><Label className="text-sm text-zinc-300">{t.settings_invite_role}</Label>
              <Select value={inviteRole} onValueChange={setInviteRole}><SelectTrigger className="bg-zinc-900 border-zinc-800 text-zinc-100"><SelectValue /></SelectTrigger>
                <SelectContent className="bg-zinc-900 border-zinc-800"><SelectItem value="admin" className="text-zinc-100">{t.settings_invite_role_admin}</SelectItem><SelectItem value="member" className="text-zinc-100">{t.settings_invite_role_member}</SelectItem><SelectItem value="viewer" className="text-zinc-100">{t.settings_invite_role_viewer}</SelectItem></SelectContent>
              </Select>
            </div>
            {inviteError && <p className="text-sm text-red-400">{inviteError}</p>}
          </div>
          <DialogFooter><Button variant="ghost" onClick={() => { setShowInviteDialog(false); resetInviteForm() }} className="text-zinc-400">{t.settings_invite_cancel}</Button>
            <button onClick={handleSendInvite} disabled={createInvite.isPending} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 disabled:opacity-50">{createInvite.isPending ? t.settings_invite_sending : t.settings_invite_send}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

type AlertFormData = { name: string; alert_type: string; threshold: string; window_minutes: string; cooldown_minutes: string; notify_roles: string[]; enabled: boolean }
const defaultAlertForm = (): AlertFormData => ({ name: '', alert_type: 'failure_rate', threshold: '10', window_minutes: '5', cooldown_minutes: '30', notify_roles: ['owner', 'admin'], enabled: true })

function AlertsTab({ orgID }: { orgID: string }) {
  const { t } = useI18n()

  const ALERT_TYPE_OPTIONS = [
    { label: t.settings_alert_type_failure_rate, value: 'failure_rate' },
    { label: t.settings_alert_type_anomalous_latency, value: 'anomalous_latency' },
    { label: t.settings_alert_type_new_failure_cluster, value: 'new_failure_cluster' },
    { label: t.settings_alert_type_error_spike, value: 'error_spike' },
  ]
  const ALL_ROLES = [
    { value: 'owner', label: t.settings_alert_role_owner },
    { value: 'admin', label: t.settings_alert_role_admin },
    { value: 'member', label: t.settings_alert_role_member },
  ]

  const { data: alerts, isLoading, error } = useAlertRules(orgID); const createAlert = useCreateAlertRule(orgID); const updateAlert = useUpdateAlertRule(orgID); const deleteAlert = useDeleteAlertRule(orgID)
  const [showAlertDialog, setShowAlertDialog] = useState(false); const [editingAlert, setEditingAlert] = useState<AlertRule | null>(null); const [formData, setFormData] = useState<AlertFormData>(defaultAlertForm()); const [alertFormError, setAlertFormError] = useState(''); const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const isFreeGated = error instanceof ApiError && error.status === 403
  function openCreate() { setEditingAlert(null); setFormData(defaultAlertForm()); setAlertFormError(''); setShowAlertDialog(true) }
  function openEdit(a: AlertRule) { setEditingAlert(a); setFormData({ name: a.name, alert_type: a.alert_type, threshold: String(a.threshold), window_minutes: String(a.window_minutes), cooldown_minutes: String(a.cooldown_minutes), notify_roles: a.notify_roles, enabled: a.enabled }); setAlertFormError(''); setShowAlertDialog(true) }
  function toggleRole(r: string) { setFormData(p => ({ ...p, notify_roles: p.notify_roles.includes(r) ? p.notify_roles.filter(x => x !== r) : [...p.notify_roles, r] })) }
  async function handleSaveAlert() { if (!formData.name.trim()) { setAlertFormError(t.auth_field_required); return }; setAlertFormError(''); const payload = { name: formData.name.trim(), alert_type: formData.alert_type, threshold: Number(formData.threshold), window_minutes: Number(formData.window_minutes), cooldown_minutes: Number(formData.cooldown_minutes), notify_roles: formData.notify_roles, enabled: formData.enabled }; try { if (editingAlert) await updateAlert.mutateAsync({ alertID: editingAlert.id, data: payload }); else await createAlert.mutateAsync(payload); setShowAlertDialog(false) } catch { setAlertFormError(t.auth_something_wrong) } }
  async function handleDeleteConfirm(id: string) { await deleteAlert.mutateAsync(id); setConfirmDelete(null) }

  if (isFreeGated) return <div className="text-center py-16"><Lock size={20} className="text-zinc-600 mx-auto mb-3" /><p className="text-sm text-zinc-500">{t.settings_alerts_free_locked}</p></div>

  return (
    <div>
      <div className="flex justify-end mb-6"><button onClick={openCreate} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-3.5 py-1.5 rounded-md hover:bg-zinc-200 transition-colors">{t.settings_alerts_add}</button></div>
      {isLoading ? <p className="text-sm text-zinc-500">{t.settings_alerts_loading}</p> : !alerts || alerts.length === 0 ? (
        <div className="text-center py-16"><p className="text-base font-medium text-zinc-200 mb-2">{t.settings_alerts_empty_title}</p><p className="text-sm text-zinc-500">{t.settings_alerts_empty_body}</p></div>
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left"><thead><tr className="border-b border-zinc-800">
              <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_alerts_col_name}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_alerts_col_type}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden sm:table-cell">{t.settings_alerts_col_threshold}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden sm:table-cell">{t.settings_alerts_col_window}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.settings_alerts_col_enabled}</th><th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider"></th>
            </tr></thead><tbody className="divide-y divide-zinc-800/40">
            {alerts.map((a: AlertRule) => (
              <tr key={a.id} className="table-row-hover">
                {confirmDelete === a.id ? (
                  <td colSpan={6} className="px-5 py-3.5"><span className="text-zinc-200 mr-4">{t.settings_alerts_delete_inline}</span><button onClick={() => handleDeleteConfirm(a.id)} className="text-sm text-red-400 mr-2">{t.settings_alerts_delete_confirm}</button><button onClick={() => setConfirmDelete(null)} className="text-sm text-zinc-500">{t.settings_alerts_delete_cancel}</button></td>
                ) : (<>
                  <td className="px-5 py-3.5 text-zinc-200 font-medium">{a.name}</td>
                  <td className="px-5 py-3.5 text-zinc-500">{ALERT_TYPE_OPTIONS.find(o => o.value === a.alert_type)?.label ?? a.alert_type}</td>
                  <td className="px-5 py-3.5 text-zinc-500 hidden sm:table-cell">{a.threshold}</td><td className="px-5 py-3.5 text-zinc-500 hidden sm:table-cell">{a.window_minutes}{t.settings_alerts_window_suffix}</td>
                  <td className="px-5 py-3.5"><span className={a.enabled ? 'text-emerald-400' : 'text-zinc-600'}>{a.enabled ? t.settings_alerts_on : t.settings_alerts_off}</span></td>
                  <td className="px-5 py-3.5"><div className="flex gap-2"><button onClick={() => openEdit(a)} className="text-sm text-zinc-500 hover:text-zinc-200">{t.settings_alerts_edit}</button><button onClick={() => setConfirmDelete(a.id)} className="text-sm text-red-400 hover:text-red-300">{t.settings_alerts_delete}</button></div></td>
                </>)}
              </tr>
            ))}
            </tbody></table>
          </div>
        </div>
      )}
      <Dialog open={showAlertDialog} onOpenChange={(o) => { if (!o) setShowAlertDialog(false) }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50 max-w-lg"><DialogHeader><DialogTitle>{editingAlert ? t.settings_alert_title_edit : t.settings_alert_title_add}</DialogTitle></DialogHeader>
          <div className="space-y-4 py-2">
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_name}</label><input value={formData.name} onChange={(e) => setFormData(p => ({ ...p, name: e.target.value }))} className={inputClass} /></div>
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_type}</label>
              <Select value={formData.alert_type} onValueChange={(v) => setFormData(p => ({ ...p, alert_type: v }))}><SelectTrigger className="bg-zinc-900 border-zinc-800 text-zinc-100"><SelectValue /></SelectTrigger>
                <SelectContent className="bg-zinc-900 border-zinc-800">{ALERT_TYPE_OPTIONS.map(o => <SelectItem key={o.value} value={o.value} className="text-zinc-100">{o.label}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_threshold}</label><input type="number" value={formData.threshold} onChange={(e) => setFormData(p => ({ ...p, threshold: e.target.value }))} className={inputClass} /></div>
              <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_window}</label><input type="number" value={formData.window_minutes} onChange={(e) => setFormData(p => ({ ...p, window_minutes: e.target.value }))} className={inputClass} /></div>
              <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_cooldown}</label><input type="number" value={formData.cooldown_minutes} onChange={(e) => setFormData(p => ({ ...p, cooldown_minutes: e.target.value }))} className={inputClass} /></div>
            </div>
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.settings_alert_notify_roles}</label><div className="flex items-center gap-3">{ALL_ROLES.map(r => (<label key={r.value} className="flex items-center gap-1.5 cursor-pointer"><input type="checkbox" checked={formData.notify_roles.includes(r.value)} onChange={() => toggleRole(r.value)} className="accent-indigo-500" /><span className="text-sm text-zinc-400 capitalize">{r.label}</span></label>))}</div></div>
            <div className="flex items-center gap-2"><input type="checkbox" id="alert-enabled" checked={formData.enabled} onChange={(e) => setFormData(p => ({ ...p, enabled: e.target.checked }))} className="accent-indigo-500" /><label htmlFor="alert-enabled" className="text-sm text-zinc-400 cursor-pointer">{t.settings_alert_enabled}</label></div>
            {alertFormError && <p className="text-sm text-red-400">{alertFormError}</p>}
          </div>
          <DialogFooter><Button variant="ghost" onClick={() => setShowAlertDialog(false)} className="text-zinc-400">{t.settings_alert_cancel}</Button>
            <button onClick={handleSaveAlert} disabled={createAlert.isPending || updateAlert.isPending} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 disabled:opacity-50">{createAlert.isPending || updateAlert.isPending ? t.settings_alert_saving : t.settings_alert_save}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export function SettingsPage() {
  const { t } = useI18n()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const [activeTab, setActiveTab] = useState<TabKey>('general')
  return (
    <div className="p-6 lg:p-8 animate-fade-in-up">
      <div className="mb-8">
        <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.settings_title}</h1>
        <p className="text-sm text-zinc-500 mt-1">{t.settings_subtitle}</p>
      </div>
      <MinimalTabs active={activeTab} onChange={setActiveTab} />
      <div className="mt-6 animate-fade-in" key={activeTab}>
        {activeTab === 'general' && <GeneralTab orgID={activeOrgID} />}
        {activeTab === 'members' && <MembersTab orgID={activeOrgID} />}
        {activeTab === 'invites' && <InvitesTab orgID={activeOrgID} />}
        {activeTab === 'alerts' && <AlertsTab orgID={activeOrgID} />}
      </div>
    </div>
  )
}
