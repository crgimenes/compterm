#!/bin/bash

# git@github.com:crgimenes/compterm.git

REPO_NAME="compterm"
GITHUB_REPO_URL="https://github.com/crgimenes/$REPO_NAME.git"
TEMP_DIR="temp_git_repo"

if [ -d "$TEMP_DIR" ]; then
    echo "Removendo diretório temporário existente..."
    rm -rf "$TEMP_DIR"
fi

mkdir "$TEMP_DIR"
cd "$TEMP_DIR"


echo "criando repositório git..."
git init

echo "clonando repositório fossil..."
fossil export --git ../../Fossilized/$REPO_NAME.fossil | git fast-import

git checkout trunk

echo "adicionando repositório remoto..."
git remote add origin "$GITHUB_REPO_URL"

# rename trunk to master
git branch -m trunk master

echo "enviando para o github..."
git push -u origin master -f

echo "removendo diretório temporário..."
cd ..
rm -rf "$TEMP_DIR"

echo "fim."



