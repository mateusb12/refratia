import {
  BookOpen,
  CheckCircle2,
  FileText,
  GitBranch,
} from 'lucide-react'

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
              <p className="mb-0 mt-2 text-base font-semibold text-primary">Versão 1.0</p>
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
        <ol className="mt-4 space-y-3 pl-0">
          {[
            ['1. Lê os documentos', 'Extrai os números dos arquivos enviados, como K, paquimetria, BAD-D e refração.'],
            ['2. Confere os números', 'Compara medidas repetidas ou relacionadas. Por exemplo: duas medidas de ACD ou a diferença esperada entre duas espessuras.'],
            ['3. Mostra quando há dúvida', 'Se os números não combinarem, o laudo marca o dado como “suspeita — revisar” e explica o motivo.'],
            ['4. Mostra de onde veio', 'Cada número usado no resultado fica ligado ao exame e à tela de onde foi extraído.'],
          ].map(([title, description]) => (
            <li className="flex gap-3 rounded-xl border border-border bg-surface-muted p-4" key={title}>
              <span className="grid h-7 w-7 flex-none place-items-center rounded-full bg-primary-soft text-xs font-bold text-primary">{title[0]}</span>
              <span><strong className="block text-text-primary">{title.slice(3)}</strong><span className="mt-1 block text-sm leading-relaxed">{description}</span></span>
            </li>
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
                ['Refratometria', 'Esfera, cilindro e eixo', 'Cálculo das dioptrias a corrigir'],
                ['Cadastro do paciente', 'Idade', 'Escolha do Fluxo A, B ou C'],
                ['Pentacam', 'K1, K2, Km; astigmatismo corneano; paquimetria; BAD-D; ARTmax; ISV, IVA, IHA, KI, CKI; TKC; coma; ACD; Z40', 'Segurança corneana, risco de ectasia, cálculos de LER/PTA, lente fácica e apoio à escolha de LIO'],
                ['Biômetro óptico (Haigis/LS900 ou equivalente)', 'K1, K2, Km e astigmatismo da biometria', 'Cálculo de K e escolha de LIO tórica ou não tórica no Fluxo C'],
                ['Microscopia especular (Nidek)', 'Contagem de células endoteliais', 'Elegibilidade para lente fácica'],
                ['OCT de retina', 'Achados da retina', 'Informação adicional no Fluxo C; não é usado nos Fluxos A e B'],
                ['Retinografia', 'Saúde macular e achados de fundo de olho', 'Observação clínica; não aprova nem bloqueia a cirurgia'],
                ['Aberrômetro (OPD-Scan III ou iTrace)', 'Aberração esférica total do olho', 'Informação para o Fluxo B; equipamento ainda não disponível na clínica'],
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
        <p className="mb-0 mt-4 text-sm">Importante: BAD-D, ARTmax e os índices de ectasia só podem ser lidos nas telas específicas do Pentacam. A tela “4 Mapas Refrativos” sozinha não é suficiente.</p>
      </ProtocolSection>

      <ProtocolSection eyebrow="FLUXO DE DECISÃO" title="Triagem por idade e cascata clínica">
        <div className="grid gap-3 sm:grid-cols-3">
          {[
            ['Fluxo A', 'Menos de 40 anos', 'Avaliação de correção refrativa e segurança corneana.'],
            ['Fluxo B', '40 a 55 anos', 'Fluxo de presbiopia, com alternativas de correção compatíveis.'],
            ['Fluxo C', '55 anos ou mais', 'Avaliação de cristalino, LIO e alternativas para o perfil do paciente.'],
          ].map(([flow, age, description]) => (
            <div className="rounded-xl border border-border bg-surface-muted p-4" key={flow}>
              <strong className="flex items-center gap-2 text-primary"><GitBranch size={16} />{flow}</strong>
              <strong className="mt-2 block text-sm text-text-primary">{age}</strong>
              <p className="mb-0 mt-1 text-sm leading-relaxed">{description}</p>
            </div>
          ))}
        </div>
        <ul className="mt-4 space-y-3 pl-0">
          <Rule>Ceratocone confirmado encerra a cascata; suspeita gera observação para revisão.</Rule>
          <Rule>PTA, LER, K final, ACD, endotélio e demais limites são avaliados apenas quando os dados necessários estão disponíveis.</Rule>
          <Rule>Coma não é bloqueante: gera observação e pode sugerir considerar abordagem topo-guiada.</Rule>
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
