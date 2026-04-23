import { Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { useI18n } from '@/i18n'

function Section({ title, body }: { title: string; body: string }) {
  return (
    <div>
      <h2 className="text-base font-medium text-zinc-200 mb-3">{title}</h2>
      {body.split('\n').map((line, i) => (
        <p key={i} className="text-sm text-zinc-400 leading-relaxed mb-1.5">
          {line}
        </p>
      ))}
    </div>
  )
}

export function PrivacyPolicyPage() {
  const { t } = useI18n()

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50 px-4 sm:px-6 py-12 sm:py-20">
      <div className="max-w-2xl mx-auto">
        <Link
          to="/"
          className="inline-flex items-center gap-1.5 text-sm text-zinc-500 hover:text-zinc-300 transition-colors mb-8"
        >
          <ArrowLeft size={14} />
          {t.legal_back_home}
        </Link>

        <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight mb-3">
          {t.legal_privacy_title}
        </h1>
        <p className="text-sm text-zinc-500 mb-10">
          {t.legal_last_updated.replace('{date}', '2026-04-09')}
        </p>

        <p className="text-sm text-zinc-400 leading-relaxed mb-10">
          {t.legal_privacy_intro}
        </p>

        <div className="space-y-8">
          <Section title={t.legal_privacy_operator_title} body={t.legal_privacy_operator_body} />
          <Section title={t.legal_privacy_data_title} body={t.legal_privacy_data_body} />
          <Section title={t.legal_privacy_purpose_title} body={t.legal_privacy_purpose_body} />
          <Section title={t.legal_privacy_storage_title} body={t.legal_privacy_storage_body} />
          <Section title={t.legal_privacy_rights_title} body={t.legal_privacy_rights_body} />
          <Section title={t.legal_privacy_cookies_title} body={t.legal_privacy_cookies_body} />
          <Section title={t.legal_privacy_changes_title} body={t.legal_privacy_changes_body} />
        </div>
      </div>
    </div>
  )
}
