#!/usr/bin/env bash

set -u
set -o pipefail

deploy-pages-refratia() {
    local usuario_destino="mateusb12"
    local repositorio="mateusb12/refratia-frontend"
    local workflow="deploy-pages.yml"
    local branch="main"

    local diretorio_projeto
    local usuario_original
    local usuario_ativo
    local branch_atual
    local commit_local
    local commit_remoto
    local run_anterior
    local run_atual

    restaurar-conta-github() {
        local codigo_saida=$?

        trap - EXIT

        if [[ -n "${usuario_original:-}" ]]; then
            usuario_ativo="$(
                gh api user --jq '.login' 2>/dev/null
            )"

            if [[ "$usuario_ativo" != "$usuario_original" ]]; then
                echo
                echo "Restaurando conta GitHub: $usuario_original"

                gh auth switch \
                    --hostname github.com \
                    --user "$usuario_original" \
                    >/dev/null 2>&1 || {
                        echo "Aviso: restaure manualmente com:"
                        echo "gh auth switch --hostname github.com --user $usuario_original"
                    }

                gh auth setup-git \
                    --hostname github.com \
                    >/dev/null 2>&1 || true
            fi
        fi

        return "$codigo_saida"
    }

    trap restaurar-conta-github EXIT

    echo "=== DEPLOY GITHUB PAGES — REFRATIA ==="
    echo

    for comando in git npm gh; do
        if ! command -v "$comando" >/dev/null 2>&1; then
            echo "Erro: comando não encontrado: $comando"
            return 1
        fi
    done

    diretorio_projeto="$(
        git rev-parse --show-toplevel 2>/dev/null
    )" || {
        echo "Erro: execute dentro do repositório RefratIA."
        return 1
    }

    cd "$diretorio_projeto" || return 1

    case "$(git remote get-url origin 2>/dev/null)" in
        "https://github.com/$repositorio.git" | \
        "git@github.com:$repositorio.git")
            ;;
        *)
            echo "Erro: o origin não aponta para $repositorio."
            git remote -v
            return 1
            ;;
    esac

    branch_atual="$(git branch --show-current)"

    if [[ "$branch_atual" != "$branch" ]]; then
        echo "Erro: o deploy deve ser executado na branch main."
        echo "Branch atual: $branch_atual"
        return 1
    fi

    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Erro: existem alterações locais não commitadas:"
        echo
        git status --short
        echo
        echo "O deploy publica apenas código já commitado."
        return 1
    fi

    usuario_original="$(
        gh api user --jq '.login' 2>/dev/null
    )" || {
        echo "Erro: não foi possível identificar a conta GitHub ativa."
        return 1
    }

    echo "Conta original: $usuario_original"
    echo "Conta destino:  $usuario_destino"
    echo

    if [[ "$usuario_original" != "$usuario_destino" ]]; then
        gh auth switch \
            --hostname github.com \
            --user "$usuario_destino" || return 1
    fi

    gh auth setup-git --hostname github.com || return 1

    usuario_ativo="$(
        gh api user --jq '.login' 2>/dev/null
    )" || return 1

    if [[ "$usuario_ativo" != "$usuario_destino" ]]; then
        echo "Erro: a conta GitHub ativa está incorreta."
        echo "Ativa:    $usuario_ativo"
        echo "Esperada: $usuario_destino"
        return 1
    fi

    echo "Conta confirmada: $usuario_ativo"
    echo

    echo "=== CONFERINDO SINCRONIA ==="

    git fetch origin "$branch" || return 1

    commit_local="$(git rev-parse HEAD)" || return 1
    commit_remoto="$(git rev-parse "origin/$branch")" || return 1

    if [[ "$commit_local" != "$commit_remoto" ]]; then
        echo "Erro: o HEAD local não corresponde ao origin/main."
        echo
        echo "Commit local:"
        git log -1 --oneline HEAD
        echo
        echo "Commit remoto:"
        git log -1 --oneline "origin/$branch"
        echo
        echo "Faça commit e push antes do deploy."
        return 1
    fi

    echo "Local e remoto sincronizados."
    echo

    echo "=== VALIDANDO INSTALAÇÃO ==="

    npm ci || return 1

    echo
    echo "=== VALIDANDO BUILD ==="

    npm run build || return 1

    echo
    echo "=== DISPARANDO WORKFLOW ==="

    run_anterior="$(
        gh run list \
            --repo "$repositorio" \
            --workflow "$workflow" \
            --branch "$branch" \
            --limit 1 \
            --json databaseId \
            --jq '.[0].databaseId // empty' \
            2>/dev/null
    )"

    gh workflow run "$workflow" \
        --repo "$repositorio" \
        --ref "$branch" || return 1

    echo "Workflow solicitado."
    echo

    for tentativa in {1..30}; do
        run_atual="$(
            gh run list \
                --repo "$repositorio" \
                --workflow "$workflow" \
                --branch "$branch" \
                --limit 1 \
                --json databaseId \
                --jq '.[0].databaseId // empty' \
                2>/dev/null
        )"

        if [[ -n "$run_atual" ]] &&
           [[ "$run_atual" != "$run_anterior" ]]; then
            break
        fi

        sleep 2
    done

    if [[ -z "$run_atual" ]] ||
       [[ "$run_atual" == "$run_anterior" ]]; then
        echo "Erro: não foi possível localizar o novo workflow."
        echo
        echo "Confira:"
        echo "https://github.com/$repositorio/actions"
        return 1
    fi

    echo "Workflow encontrado: $run_atual"
    echo
    echo "Acompanhando deploy..."

    gh run watch "$run_atual" \
        --repo "$repositorio" \
        --exit-status || {
            echo
            echo "O deploy falhou."
            echo
            echo "Logs:"
            echo "gh run view $run_atual --repo $repositorio --log-failed"
            return 1
        }

    echo
    echo "=== DEPLOY CONCLUÍDO ==="
    echo
    echo "Site:"
    echo "https://mateusb12.github.io/refratia-frontend/"
    echo
    echo "Commit publicado:"
    git log -1 --oneline
}

deploy-pages-refratia
