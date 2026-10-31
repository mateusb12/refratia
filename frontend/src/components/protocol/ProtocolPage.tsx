import { Fragment } from 'react'
import {
  BookOpen,
  CheckCircle2,
  FileText,
  GitBranch,
  ArrowRight,
} from 'lucide-react'
import { formatLongDatePtBR } from '../../utils/date'

const protocolRevisionDate = '2026-10-17T00:00:00Z'

function ProtocolSection({
  eyebrow,
  title,
  children,
}: {
  eyebrow: string
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <span className="text-primary text-xs font-bold tracking-[0.13em]">{eyebrow}</span>
      <h2 className="mb-0 mt-1 font-display text-xl">{title}</h2>
      <div className="mt-4 text-base leading-relaxed text-text-secondary">{children}</div>
    </section>
  )
}

function Rule({ children }: { children: React.ReactNode }) {
  return (
    <li className="flex gap-2.5">
      <CheckCircle2 className="mt-0.5 flex-none text-primary" size={16} />
      <span>{children}</span>
    </li>
  )
}

export default function ProtocolPage() {
  return (
    <div className="mt-5 space-y-5">
      <section className="rounded-2xl border border-primary-border bg-primary-soft/30 p-6 shadow-sm max-[580px]:p-4">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex gap-3">
            <span className="grid h-11 w-11 flex-none place-items-center rounded-xl bg-primary-soft text-primary">
              <BookOpen size={23} />
            </span>
            <div>
              <span className="text-primary text-xs font-bold tracking-[0.13em]">DOCUMENTO CLÍNICO</span>
              <h2 className="mb-0 mt-1 font-display text-2xl">Protocolo RefratIA</h2>
              <p className="mb-0 mt-2 text-base font-semibold text-primary">Revisão de {formatLongDatePtBR(protocolRevisionDate)} · itens 1 a 78</p>
            </div>
          </div>
        </div>
      </section>

      <ProtocolSection eyebrow="PRINCÍPIO ABSOLUTO" title="Nenhum dado é inventado">
        <p className="mt-0">Todo valor usado no cálculo precisa ser identificado como dado bruto, extraído diretamente do exame, ou dado calculado por uma fórmula definida no protocolo.</p>
        <ul className="mt-4 space-y-3 pl-0">
          <Rule>Se um dado necessário não estiver disponível, ele é marcado como <strong>dado ausente</strong>.</Rule>
          <Rule>O sistema não estima, presume valores típicos nem reaproveita cálculo antigo quando a entrada mudou.</Rule>
          <Rule>O laudo registra os dados usados, a origem de cada valor e as pendências que limitam a confiança.</Rule>
        </ul>
      </ProtocolSection>

      <ProtocolSection eyebrow="COMO O SISTEMA CONFERE OS EXAMES" title="Leitura e conferência dos dados">
        <p className="mt-0">O sistema segue quatro passos para reduzir erros de leitura:</p>
        <ol className="mx-auto mt-4 grid w-full max-w-[940px] gap-3 pl-0 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto_minmax(0,1fr)_auto_minmax(0,1fr)] lg:items-stretch lg:gap-x-3">
          {[
            ['1. Lê os documentos', 'Extrai os números dos arquivos enviados, como K, paquimetria, BAD-D e refração.'],
            ['2. Confere os números', 'Compara medidas repetidas ou relacionadas. Por exemplo: duas medidas de ACD ou a diferença esperada entre duas espessuras.'],
            ['3. Mostra quando há dúvida', 'Se os números não combinarem, o laudo marca o dado como “suspeita — revisar” e explica o motivo.'],
            ['4. Mostra de onde veio', 'Cada número usado no resultado fica ligado ao exame e à tela de onde foi extraído.'],
          ].map(([title, description], index) => (
            <Fragment key={title}>
              <li className="flex min-h-[220px] flex-col rounded-xl border border-border bg-surface-muted p-4">
              <span className="mx-auto grid h-7 w-7 flex-none place-items-center rounded-full bg-primary-soft text-xs font-bold text-primary">{title[0]}</span>
              <span className="mt-3"><strong className="block text-text-primary">{title.slice(3)}</strong><span className="mt-1 block text-sm leading-relaxed">{description}</span></span>
              </li>
              {index < 3 && <li aria-hidden="true" className="hidden items-center justify-center lg:flex"><ArrowRight className="text-primary" size={20} /></li>}
            </Fragment>
          ))}
        </ol>
        <div className="mt-4 rounded-xl border border-border bg-surface p-4 text-sm leading-relaxed text-text-secondary">
          <strong className="block text-sm text-danger">Quando o sistema para?</strong>
          <span className="mt-1 block">Somente quando não consegue confirmar que todos os documentos pertencem ao mesmo paciente. Uma dúvida sobre um número clínico é mostrada para revisão, mas não interrompe o fluxo.</span>
        </div>
      </ProtocolSection>

      <ProtocolSection eyebrow="DADOS E EXAMES" title="Quais dados entram e de onde vêm">
        <p className="mt-0">O protocolo não trata todos os exames da mesma forma. Cada dado tem uma fonte definida e um uso específico:</p>
        <div className="mt-4 overflow-x-auto rounded-xl border border-border">
          <table className="w-full min-w-[760px] border-collapse text-left text-sm">
            <thead className="bg-surface-muted text-text-secondary">
              <tr>
                <th className="px-4 py-3 font-bold">Exame ou fonte</th>
                <th className="px-4 py-3 font-bold">Dados lidos</th>
                <th className="px-4 py-3 font-bold">Para que servem</th>
              </tr>
            </thead>
            <tbody>
              {[
                ['Refratometria', 'Esfera, cilindro e eixo; registrar se foi dinâmica, estática ou autorrefratometria', 'Equivalente esférico, K final e cálculo da ablação'],
                ['Cadastro / identificação do paciente', 'Idade, nome, nascimento e datas dos documentos', 'Escolha do fluxo, conferência de identidade e janela máxima de 4 meses entre exames'],
                ['Pentacam', 'K1, K2, Km; paquimetria; BAD-D; ARTmax; ISV, IVA, IHA, KI, CKI, IHD; TKC; ACD; coma em 4 mm; Z40; PNS; QI', 'Segurança corneana, risco de ectasia, LER/PTA, lente fácica e informações para LIO'],
                ['Biômetro óptico (Haigis/LS900 ou equivalente)', 'K1, K2, Km e astigmatismo da biometria', 'Cálculo de K e escolha de LIO tórica ou não tórica no Fluxo C'],
                ['Microscopia especular (Nidek)', 'Contagem de células endoteliais', 'Elegibilidade para lente fácica e condição da indicação no Fluxo C'],
                ['OCT de retina', 'Achados da retina', 'Informação adicional no Fluxo C; não altera a indicação cirúrgica'],
                ['Retinografia', 'Saúde macular e achados de fundo de olho', 'Observação clínica; não aprova nem bloqueia a cirurgia'],
                ['Aberrômetro iTrace', 'Aberração esférica total do olho', 'Informação para o Fluxo B; equipamento ainda não disponível na clínica'],
                ['Pupilômetro / equipamento equivalente', 'Pupila escotópica', 'Informação sobre qualidade visual noturna, quando disponível'],
              ].map(([source, data, use]) => (
                <tr className="border-t border-border align-top" key={source}>
                  <td className="px-4 py-3 font-bold text-text-primary">{source}</td>
                  <td className="px-4 py-3 text-text-secondary">{data}</td>
                  <td className="px-4 py-3 text-text-secondary">{use}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mb-0 mt-4 text-sm">Importante: BAD-D, ARTmax e os índices de ectasia só podem ser lidos nas telas específicas do Pentacam. A tela “4 Mapas Refrativos” sozinha não é suficiente. Exames de equipamentos não listados podem ser usados para conferir identidade e comparar medidas, mas não substituem as fontes oficiais.</p>
      </ProtocolSection>

      <ProtocolSection eyebrow="FLUXO DE DECISÃO" title="Triagem por idade e cascata clínica">
        <div className="grid gap-3 sm:grid-cols-3">
          {[
            ['Fluxo A', '18 a 39 anos', 'Avaliação de correção refrativa, córnea e lente fácica.'],
            ['Fluxo B', '40 a 54 anos', 'Fluxo de presbiopia, com alternativas de correção compatíveis.'],
            ['Fluxo C', '55 anos ou mais', 'Avaliação de cristalino, LIO e alternativas para o perfil do paciente.'],
          ].map(([flow, age, description]) => (
            <div className="rounded-xl border border-border bg-surface-muted p-4" key={flow}>
              <strong className="flex items-center gap-2 text-primary"><GitBranch size={16} />{flow}</strong>
              <strong className="mt-2 block text-sm text-text-primary">{age}</strong>
              <p className="mb-0 mt-1 text-sm leading-relaxed">{description}</p>
            </div>
          ))}
        </div>
        <p className="mb-0 mt-4">Pacientes com menos de 18 anos ficam fora do escopo do protocolo.</p>
        <ul className="mt-4 space-y-3 pl-0">
          <Rule>Ceratocone confirmado encerra a cascata; suspeita gera observação para revisão.</Rule>
          <Rule>PTA, LER, K final, ACD, endotélio e demais limites são avaliados apenas quando os dados necessários estão disponíveis.</Rule>
          <Rule>Coma não é bloqueante: o valor é registrado, sem aplicar limite ou reprovar a conduta.</Rule>
        </ul>
      </ProtocolSection>

      <ProtocolSection eyebrow="REGRAS DE PARADA" title="Quando o protocolo bloqueia ou fica pendente">
        <div className="mx-auto grid w-full max-w-[940px] gap-4 sm:grid-cols-2">
          {[
            ['Bloqueia o caso', 'Os documentos não podem ser confirmados como pertencentes ao mesmo paciente.', 'danger'],
            ['Fica pendente', 'Falta um exame necessário ou ele está fora da janela de 4 meses em relação ao Pentacam.', 'warning'],
            ['Apenas registra', 'Uma dúvida em um número clínico, ou um dado informativo como coma, PNS, Z40, OCT e retinografia, gera observação e não interrompe o fluxo.', 'neutral'],
            ['Fora do escopo', 'Pacientes com menos de 18 anos não entram na cascata automática do protocolo.', 'neutral'],
          ].map(([title, description, tone]) => (
            <div className="rounded-xl border border-border bg-surface-muted p-4" key={title}>
              <strong className={[
                'block text-sm',
                tone === 'danger' ? 'text-danger' : tone === 'warning' ? 'text-warning' : 'text-primary',
              ].join(' ')}>{title}</strong>
              <p className="mb-0 mt-2 text-sm leading-relaxed">{description}</p>
            </div>
          ))}
        </div>
      </ProtocolSection>

      <ProtocolSection eyebrow="MUDANÇAS DESTA REVISÃO" title="O que mudou no protocolo">
        <ul className="mt-0 space-y-3 pl-0">
          <Rule>O coma deixou de ter limite de aprovação ou reprovação. Agora é apenas registrado, na zona de 4 mm, sem bloquear a conduta.</Rule>
          <Rule>LASIK e PRK passam a ser calculados juntos abaixo de 12 D. Não existe mais corte de 6 D separando as duas técnicas; a escolha fica com o cirurgião.</Rule>
          <Rule>A espessura do flap do LASIK passou para 90 µm.</Rule>
          <Rule>A profundidade da ablação passou a usar a fórmula de Munnerlyn, com a zona óptica como variável.</Rule>
          <Rule>PNS, Z40 e pupila escotópica passaram a ser registrados como informações, sem criar bloqueios por si só.</Rule>
        </ul>
      </ProtocolSection>

      <section className="rounded-2xl border border-border bg-surface-muted p-5 text-sm text-text-secondary">
        <div className="flex items-start gap-3">
          <FileText className="mt-0.5 flex-none text-primary" size={18} />
          <div>
            <strong className="text-text-primary">Fonte do documento</strong>
            <p className="mb-0 mt-1">Esta página resume o documento <em>Protocolo_RefratIA_FINAL.pdf</em> e as regras implementadas no motor atual. Qualquer alteração clínica deve atualizar o documento-fonte e a implementação correspondente.</p>
          </div>
        </div>
      </section>
    </div>
  )
}
