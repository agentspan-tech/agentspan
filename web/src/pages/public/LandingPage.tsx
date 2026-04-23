import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, Activity, Shield, Layers, Globe, Key, Radio, BarChart2, Bell, Zap, Eye, Package, Check, X } from 'lucide-react'
import { LandingNav } from '@/components/app/LandingNav'
import { IntegrationCodeBlock } from '@/components/landing/IntegrationCodeBlock'
import { ProRequestDialog } from '@/components/app/ProRequestDialog'
import { useI18n } from '@/i18n'

function Divider() {
  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6">
      <div className="border-t border-zinc-900" />
    </div>
  )
}

export function LandingPage() {
  const { t } = useI18n()
  const [proDialogOpen, setProDialogOpen] = useState(false)

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <LandingNav />

      <main>
      {/* Hero */}
      <section className="pt-28 sm:pt-40 pb-20 sm:pb-28 px-4 sm:px-6">
        <div className="max-w-3xl mx-auto">
          <span className="inline-block text-xs font-medium uppercase tracking-widest text-indigo-400 bg-indigo-500/10 border border-indigo-500/20 px-3 py-1 rounded-full mb-8">
            {t.hero_badge}
          </span>

          <h1 className="text-3xl sm:text-5xl md:text-6xl font-semibold tracking-[-0.03em] leading-[1.08] mb-5 sm:mb-6 text-zinc-50">
            {t.hero_title}{' '}
            <span className="text-zinc-500">{t.hero_title_highlight}</span>
          </h1>

          <p className="text-base sm:text-lg text-zinc-400 leading-relaxed mb-10 sm:mb-12 max-w-xl">
            {t.hero_subtitle}
          </p>

          <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
            <Link
              to="/register"
              className="inline-flex items-center gap-2 text-sm font-medium bg-zinc-50 text-zinc-950 px-5 py-2.5 rounded-md hover:bg-zinc-200 transition-colors duration-150 group btn-press"
            >
              {t.hero_cta}
              <ArrowRight size={14} className="group-hover:translate-x-0.5 transition-transform duration-150" />
            </Link>
            <a
              href="https://github.com/agentorbit-tech/agentorbit/tree/main/docs"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center text-sm text-zinc-500 hover:text-zinc-200 transition-colors duration-150 px-5 py-2.5"
            >
              {t.hero_docs}
            </a>
          </div>
        </div>

        {/* Dashboard preview mockup */}
        <div className="mt-14 sm:mt-20 max-w-5xl mx-auto">
          <div className="rounded-lg border border-zinc-800 overflow-hidden shadow-2xl shadow-black/40">
            <div className="flex items-center gap-1.5 px-4 py-2.5 border-b border-zinc-800 bg-zinc-900">
              <span className="w-2 h-2 rounded-full bg-zinc-700" />
              <span className="w-2 h-2 rounded-full bg-zinc-700" />
              <span className="w-2 h-2 rounded-full bg-zinc-700" />
              <span className="ml-4 text-[10px] text-zinc-600 font-mono">agentorbit.tech/dash</span>
            </div>
            <div className="flex bg-zinc-950">
              {/* Mini sidebar */}
              <div className="hidden sm:flex w-11 border-r border-zinc-800/60 flex-col items-center pt-3 gap-2 shrink-0">
                <div className="w-4 h-4 rounded bg-zinc-800 mb-1" />
                <div className="w-4 h-4 rounded bg-zinc-800 ring-1 ring-zinc-700" />
                <div className="w-4 h-4 rounded bg-zinc-900" />
                <div className="w-4 h-4 rounded bg-zinc-900" />
              </div>

              <div className="flex-1 p-4 sm:p-5">
                {/* Header */}
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] font-medium text-zinc-300">Dashboard</span>
                    <span className="relative flex h-1.5 w-1.5">
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-60" />
                      <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500" />
                    </span>
                  </div>
                  <div className="flex">
                    {['24h', '7d', '30d'].map((p, i) => (
                      <span key={p} className={`text-[9px] px-2 py-0.5 ${i === 1 ? 'bg-zinc-800 text-zinc-200' : 'text-zinc-600'} ${i === 0 ? 'rounded-l' : i === 2 ? 'rounded-r' : ''}`}>{p}</span>
                    ))}
                  </div>
                </div>

                {/* KPI cards */}
                <div className="grid grid-cols-4 gap-2 mb-3">
                  {[
                    { label: 'Total sessions', value: '284' },
                    { label: 'Failure rate', value: '8.4%' },
                    { label: 'Avg duration', value: '1.8s' },
                    { label: 'Total tokens', value: '1.2M' },
                  ].map((stat) => (
                    <div key={stat.label} className="rounded-md border border-zinc-800 bg-zinc-900 p-2.5">
                      <div className="text-[8px] text-zinc-500 mb-1 truncate">{stat.label}</div>
                      <div className="text-sm font-semibold leading-none tabular-nums">{stat.value}</div>
                    </div>
                  ))}
                </div>

                {/* Chart + sessions */}
                <div className="flex gap-2 min-h-0">
                  <div className="flex-1 rounded-md border border-zinc-800 bg-zinc-900 p-2.5 flex flex-col">
                    <div className="text-[8px] text-zinc-500 mb-2">Sessions over time</div>
                    <div className="flex items-end gap-[2px] h-20 flex-1">
                      {[65, 48, 72, 55, 80, 42, 90, 58, 74, 62].map((h, i) => (
                        <div key={i} className="flex-1 flex flex-col justify-end gap-[1px]" style={{ height: '100%' }}>
                          <div className="w-full rounded-[1px] bg-red-500/25" style={{ height: `${h * 0.12}%` }} />
                          <div className="w-full rounded-[1px] bg-zinc-600" style={{ height: `${h * 0.88}%` }} />
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className="flex-1 rounded-md border border-zinc-800 bg-zinc-900 p-2.5 flex flex-col">
                    <div className="text-[8px] text-zinc-500 mb-2">Recent sessions</div>
                    <div className="flex flex-col gap-1 flex-1">
                      <div className="grid grid-cols-[1fr_1.5fr_auto] gap-1 pb-1 border-b border-zinc-800">
                        {['Status', 'Category', 'Dur.'].map(h => (
                          <span key={h} className="text-[7px] text-zinc-600 font-medium">{h}</span>
                        ))}
                      </div>
                      {[
                        { status: 'Completed', cat: 'Q&A', dur: '1.2s', color: 'text-emerald-400' },
                        { status: 'Failed', cat: 'Generation', dur: '3.8s', color: 'text-red-400' },
                        { status: 'Completed', cat: 'Summary', dur: '0.9s', color: 'text-emerald-400' },
                        { status: 'Abandoned', cat: 'Tool call', dur: '8.1s', color: 'text-zinc-400' },
                      ].map((s, i) => (
                        <div key={i} className="grid grid-cols-[1fr_1.5fr_auto] gap-1 items-center py-0.5">
                          <span className={`text-[7px] font-medium truncate ${s.color}`}>{s.status}</span>
                          <span className="text-[7px] text-zinc-500 truncate">{s.cat}</span>
                          <span className="text-[7px] text-zinc-500 font-mono">{s.dur}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <Divider />

      {/* Competitive advantages */}
      <section className="py-16 sm:py-24 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-10 sm:gap-8 lg:gap-12">
            {[
              { icon: Bell, title: t.adv_alerts_title, desc: t.adv_alerts_desc, extra: null },
              { icon: Package, title: t.adv_nosdk_title, desc: t.adv_nosdk_desc, extra: null },
              { icon: Zap, title: t.adv_latency_title, desc: t.adv_latency_desc, extra: (
                <p className="text-4xl sm:text-5xl font-semibold tracking-[-0.04em] text-zinc-200 mb-3 tabular-nums">
                  59<span className="text-xl sm:text-2xl text-zinc-600 ml-0.5">ms</span>
                </p>
              )},
              { icon: Eye, title: t.adv_readable_title, desc: t.adv_readable_desc, extra: null },
            ].map(({ icon: Icon, title, desc, extra }) => (
              <div key={title}>
                <Icon size={16} className="text-zinc-600 mb-4" />
                {extra}
                <h3 className="text-sm font-medium text-zinc-200 mb-2">{title}</h3>
                <p className="text-sm text-zinc-500 leading-relaxed">{desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <Divider />

      {/* Compatibility */}
      <section className="py-16 sm:py-24 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <div className="mb-12 sm:mb-16">
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-[-0.02em] mb-3">{t.compat_title}</h2>
            <p className="text-sm text-zinc-400 max-w-md">{t.compat_subtitle}</p>
          </div>

          {/* Compatible */}
          <div className="mb-8">
            <div className="flex items-center gap-2 mb-5">
              <div className="flex items-center justify-center w-5 h-5 rounded-full bg-emerald-500/10">
                <Check size={12} className="text-emerald-500" />
              </div>
              <span className="text-sm font-medium text-zinc-300">{t.compat_works}</span>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {[
                { name: t.compat_openclaw, desc: t.compat_openclaw_desc },
                { name: t.compat_n8n, desc: t.compat_n8n_desc },
                { name: t.compat_cursor, desc: t.compat_cursor_desc },
                { name: t.compat_crewai, desc: t.compat_crewai_desc },
              ].map((agent) => (
                <div key={agent.name} className="border border-zinc-800 rounded-lg p-4 sm:p-5">
                  <div className="flex flex-wrap items-center gap-1.5 sm:gap-2 mb-1.5">
                    <span className="text-sm font-medium text-zinc-100">{agent.name}</span>
                    <span className="text-[10px] font-medium uppercase tracking-wider text-emerald-500/80 bg-emerald-500/10 px-1.5 py-0.5 rounded">
                      {t.compat_via_url}
                    </span>
                  </div>
                  <p className="text-sm text-zinc-500 leading-relaxed">{agent.desc}</p>
                </div>
              ))}
            </div>
          </div>

          {/* Incompatible */}
          <div>
            <div className="flex items-center gap-2 mb-5">
              <div className="flex items-center justify-center w-5 h-5 rounded-full bg-zinc-800">
                <X size={12} className="text-zinc-500" />
              </div>
              <span className="text-sm font-medium text-zinc-500">{t.compat_no}</span>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {[
                { name: t.compat_claude_code, desc: t.compat_claude_code_desc },
                { name: t.compat_copilot, desc: t.compat_copilot_desc },
              ].map((agent) => (
                <div key={agent.name} className="border border-zinc-800/40 rounded-lg p-4 sm:p-5 opacity-60">
                  <div className="flex flex-wrap items-center gap-1.5 sm:gap-2 mb-1.5">
                    <span className="text-sm font-medium text-zinc-300">{agent.name}</span>
                    <span className="text-[10px] font-medium uppercase tracking-wider text-zinc-500 bg-zinc-800 px-1.5 py-0.5 rounded">
                      {t.compat_closed_sdk}
                    </span>
                  </div>
                  <p className="text-sm text-zinc-500 leading-relaxed">{agent.desc}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      <Divider />

      {/* How it works */}
      <section id="how-it-works" className="py-16 sm:py-24 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <div className="mb-12 sm:mb-16">
            <p className="text-xs font-medium text-zinc-500 uppercase tracking-widest mb-4">{t.how_badge}</p>
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-[-0.02em] mb-3">{t.how_title}</h2>
            <p className="text-sm text-zinc-400 max-w-md">{t.how_subtitle}</p>
          </div>

          {/* Steps */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-10 sm:gap-8 mb-14">
            {[
              { icon: Key, num: '1', title: t.how_step1_title, desc: t.how_step1_desc },
              { icon: Radio, num: '2', title: t.how_step2_title, desc: t.how_step2_desc },
              { icon: BarChart2, num: '3', title: t.how_step3_title, desc: t.how_step3_desc },
            ].map(({ icon: Icon, num, title, desc }) => (
              <div key={num}>
                <div className="flex items-baseline gap-3 mb-4">
                  <span className="text-2xl font-semibold text-zinc-800 tabular-nums">{num}</span>
                  <Icon size={14} className="text-zinc-500" />
                </div>
                <h3 className="text-base font-medium text-zinc-100 mb-2">{title}</h3>
                <p className="text-sm text-zinc-400 leading-relaxed">{desc}</p>
              </div>
            ))}
          </div>

          {/* Code block */}
          <IntegrationCodeBlock
            labels={{
              openai: t.how_tab_openai,
              anthropic: t.how_tab_anthropic,
              curl: t.how_tab_curl,
            }}
            copyLabel={t.how_copy}
            copiedLabel={t.how_copied}
          />

          {/* Architecture diagram */}
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="flex items-center gap-3 p-4 rounded-lg border border-zinc-800 bg-zinc-900">
              <Zap size={14} className="text-zinc-400 shrink-0" />
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-zinc-100">{t.how_arch_sync}</span>
                <span className="text-sm text-zinc-500"> — {t.how_arch_sync_desc}</span>
              </div>
              <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-mono border bg-emerald-950 text-emerald-500 border-emerald-900">
                {t.how_arch_latency}
              </span>
            </div>
            <div className="flex items-center gap-3 p-4 rounded-lg border border-zinc-800 bg-zinc-900">
              <Activity size={14} className="text-zinc-400 shrink-0" />
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-zinc-100">{t.how_arch_async}</span>
                <span className="text-sm text-zinc-500"> — {t.how_arch_async_desc}</span>
              </div>
              <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-mono border bg-amber-950 text-amber-500 border-amber-900">
                {t.how_arch_async_label}
                </span>
            </div>
          </div>
        </div>
      </section>

      <Divider />

      {/* Features */}
      <section id="features" className="py-16 sm:py-24 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <div className="mb-12 sm:mb-16">
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-[-0.02em] mb-3">{t.features_title}</h2>
            <p className="text-sm text-zinc-400 max-w-md">{t.features_subtitle}</p>
          </div>

          {/* Session Timeline — full width, the primary feature */}
          <div className="border border-zinc-800 rounded-lg p-6 mb-3">
            <div className="sm:flex sm:items-start sm:gap-10">
              <div className="mb-5 sm:mb-0 sm:flex-1">
                <Activity size={15} className="text-zinc-600 mb-3" />
                <h3 className="text-base font-medium text-zinc-200 mb-2">{t.feat_timeline_title}</h3>
                <p className="text-sm text-zinc-400 leading-relaxed">{t.feat_timeline_desc}</p>
              </div>
              {/* Mini timeline preview */}
              <div className="border-l border-zinc-800 pl-4 space-y-2.5 shrink-0 sm:w-56">
                {[
                  { time: '14:23:01', model: 'gpt-4o', tokens: '1.2k → 340', status: 200 },
                  { time: '14:23:03', model: 'gpt-4o', tokens: '890 → 120', status: 200 },
                  { time: '14:23:05', model: 'gpt-4o', tokens: '2.1k → 0', status: 500 },
                ].map((span, i) => (
                  <div key={i} className="relative flex items-center justify-between text-[10px]">
                    <div className={`absolute -left-[18px] top-1/2 -translate-y-1/2 w-1.5 h-1.5 rounded-full ${span.status === 200 ? 'bg-zinc-600' : 'bg-red-500'}`} />
                    <span className="font-mono text-zinc-600">{span.time}</span>
                    <span className="text-zinc-500">{span.model}</span>
                    <span className={span.status === 200 ? 'text-zinc-600' : 'text-red-400'}>{span.status}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* Other features */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {[
              { icon: Shield, title: t.feat_clusters_title, desc: t.feat_clusters_desc },
              { icon: Layers, title: t.feat_categories_title, desc: t.feat_categories_desc },
              { icon: Globe, title: t.feat_providers_title, desc: t.feat_providers_desc },
            ].map(({ icon: Icon, title, desc }) => (
              <div key={title} className="border border-zinc-800 rounded-lg p-6">
                <Icon size={15} className="text-zinc-600 mb-3" />
                <h3 className="text-sm font-medium text-zinc-200 mb-2">{title}</h3>
                <p className="text-sm text-zinc-400 leading-relaxed">{desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <Divider />

      {/* Pricing */}
      <section id="pricing" className="py-16 sm:py-24 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <div className="mb-12 sm:mb-16">
            <h2 className="text-2xl sm:text-3xl font-semibold tracking-[-0.02em] mb-3">{t.pricing_title}</h2>
            <p className="text-sm text-zinc-400 max-w-md">{t.pricing_beta}</p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-10 sm:gap-8">
            {/* Free */}
            <div className="flex flex-col">
              <h3 className="text-sm font-medium text-zinc-400 mb-1">{t.pricing_free}</h3>
              <div className="text-2xl font-semibold text-zinc-100 mb-1">{t.pricing_free_price}</div>
              <p className="text-sm text-zinc-500 mb-6">{t.pricing_free_desc}</p>
              <ul className="space-y-2 mb-8 flex-1">
                {[t.pricing_free_f1, t.pricing_free_f2, t.pricing_free_f3, t.pricing_free_f4, t.pricing_free_f5, t.pricing_free_f6].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-zinc-500">
                    <Check size={13} className="text-zinc-700 shrink-0" />
                    {f}
                  </li>
                ))}
              </ul>
              <Link
                to="/register"
                className="w-full text-center text-sm font-medium bg-zinc-800 text-zinc-200 px-5 py-2.5 rounded-md hover:bg-zinc-700 transition-colors duration-150 block btn-press"
              >
                {t.pricing_free_cta}
              </Link>
            </div>

            {/* Pro */}
            <div className="flex flex-col sm:border-x sm:border-zinc-800/60 sm:px-8">
              <div className="flex items-center gap-2 mb-1">
                <h3 className="text-sm font-medium text-zinc-200">{t.pricing_pro}</h3>
                <span className="text-[10px] font-medium uppercase tracking-wider text-indigo-400 bg-indigo-500/10 px-1.5 py-0.5 rounded">
                  {t.pricing_beta}
                </span>
              </div>
              <div className="text-2xl font-semibold text-zinc-50 mb-1">{t.pricing_pro_price}</div>
              <p className="text-sm text-zinc-400 mb-6">{t.pricing_pro_desc}</p>
              <ul className="space-y-2 mb-8 flex-1">
                {[t.pricing_pro_f1, t.pricing_pro_f2, t.pricing_pro_f3, t.pricing_pro_f4, t.pricing_pro_f5, t.pricing_pro_f6, t.pricing_pro_f7, t.pricing_pro_f8].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-zinc-300">
                    <Check size={13} className="text-indigo-400 shrink-0" />
                    {f}
                  </li>
                ))}
              </ul>
              <button
                onClick={() => setProDialogOpen(true)}
                className="w-full text-center text-sm font-medium bg-zinc-50 text-zinc-950 px-5 py-2.5 rounded-md hover:bg-zinc-200 transition-colors duration-150 block btn-press"
              >
                {t.pricing_pro_cta}
              </button>
            </div>

            {/* Self-host */}
            <div className="flex flex-col">
              <h3 className="text-sm font-medium text-zinc-200 mb-1">{t.pricing_selfhost}</h3>
              <div className="text-2xl font-semibold text-zinc-100 mb-1">{t.pricing_selfhost_price}</div>
              <p className="text-sm text-zinc-500 mb-6">{t.pricing_selfhost_desc}</p>
              <ul className="space-y-2 mb-8 flex-1">
                {[t.pricing_selfhost_f1, t.pricing_selfhost_f2, t.pricing_selfhost_f3, t.pricing_selfhost_f4, t.pricing_selfhost_f5, t.pricing_selfhost_f6, t.pricing_selfhost_f7, t.pricing_selfhost_f8].map((f) => (
                  <li key={f} className="flex items-center gap-2 text-sm text-zinc-400">
                    <Check size={13} className="text-zinc-600 shrink-0" />
                    {f}
                  </li>
                ))}
              </ul>
              <a
                href="https://github.com/agentorbit-tech/agentorbit"
                target="_blank"
                rel="noopener noreferrer"
                className="w-full text-center text-sm font-medium bg-zinc-800 text-zinc-200 px-5 py-2.5 rounded-md hover:bg-zinc-700 transition-colors duration-150 block btn-press"
              >
                {t.pricing_selfhost_cta}
              </a>
            </div>
          </div>
        </div>
      </section>

      <Divider />

      {/* CTA */}
      <section className="py-20 sm:py-28 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto">
          <h2 className="text-2xl sm:text-3xl font-semibold tracking-[-0.02em] mb-3">{t.cta_title}</h2>
          <p className="text-sm text-zinc-400 mb-8 max-w-sm">{t.cta_subtitle}</p>
          <Link
            to="/register"
            className="inline-flex items-center gap-2 text-sm font-medium bg-zinc-50 text-zinc-950 px-5 py-2.5 rounded-md hover:bg-zinc-200 transition-colors duration-150 group btn-press"
          >
            {t.hero_cta}
            <ArrowRight size={14} className="group-hover:translate-x-0.5 transition-transform duration-150" />
          </Link>
        </div>
      </section>

      </main>

      <Divider />

      {/* Footer */}
      <footer className="py-10 px-4 sm:px-6">
        <div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-6">
            <span className="text-sm font-semibold text-zinc-50">AgentOrbit</span>
            <span className="text-xs text-zinc-500">{t.footer_copyright}</span>
          </div>
          <div className="flex items-center gap-6 text-xs text-zinc-500">
            <a href="/privacy-policy" className="hover:text-zinc-400 transition-colors duration-150">{t.footer_privacy}</a>
            <a href="/terms" className="hover:text-zinc-400 transition-colors duration-150">{t.footer_terms}</a>
            <a href="/consent" className="hover:text-zinc-400 transition-colors duration-150">{t.footer_consent}</a>
          </div>
        </div>
      </footer>

      <ProRequestDialog open={proDialogOpen} onOpenChange={setProDialogOpen} source="landing_pricing" />
    </div>
  )
}
