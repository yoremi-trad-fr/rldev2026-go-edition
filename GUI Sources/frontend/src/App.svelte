<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime.js';
  import {
    SelectFile,
    SelectDirectory,
    SelectSaveFile,
    StopProcess,
    RldevDisassemble,
    RldevExtract,
    RldevArchive,
    RldevList,
    RldevCompile,
    RldevCompileBatch,
    RldevG00ToPng,
    RldevPngToG00,
    RldevGanToXml,
    RldevXmlToGan
  } from '../wailsjs/go/main/App.js';

  let rldevSelectedOp = 'kprl_disasm';
  let running = false;
  let consoleLines = [];
  let consoleEl;

  let rlSeenFile = '';
  let rlOrgFile = '';
  let rlOrgDir = '';
  let rlCompileBatch = false;
  let rlKfnFile = '';
  let rlGameexe = '';
  let rlInterpreter = '';
  let rlOutputDir = '';
  let rlEncoding = 'UTF-8';
  let rlOutputTransform = 'WESTERN';
  let rlForceTransform = true;
  let rlGameId = '';
  let rlG00File = '';
  let rlPngFile = '';
  let rlGanFile = '';

  const rldevOperations = [
    { id: '_rs1', label: 'KPRL / RLC', section: true },
    { id: 'kprl_list',    label: '1 — List SEEN.txt' },
    { id: 'kprl_disasm',  label: '2 — Extract SEEN.txt' },
    { id: 'rlc_compile',  label: '3 — Compile .org / .ke' },
    { id: 'kprl_archive', label: '4 — Rebuild SEEN.txt' },
    { id: 'kprl_extract', label: 'Extract raw / uncompressed' },
    { id: '_rs3', label: 'IMAGE (G00)', section: true },
    { id: 'g00_extract', label: 'G00 → PNG' },
    { id: 'g00_import', label: 'PNG → G00' },
    { id: '_rs4', label: 'ANIMATION (GAN)', section: true },
    { id: 'gan_to_xml', label: 'GAN → XML' },
    { id: 'gan_from_xml', label: 'XML → GAN' }
  ];

  let pendingLines = [];
  let flushTimer = null;
  const maxConsoleLines = 12000;
  const keepConsoleLines = 10000;

  function addLine(text) {
    let cls = '';
    if (text.includes('[OK]')) cls = 'line-ok';
    else if (text.includes('[ERROR]') || text.includes('Error')) cls = 'line-err';
    else if (text.includes('Warning')) cls = 'line-warn';
    else if (text.startsWith('═') || text.startsWith('─')) cls = 'line-sep';
    else if (text.startsWith('>')) cls = 'line-cmd';
    pendingLines.push({ text, cls });
    if (!flushTimer) flushTimer = setTimeout(flushConsole, 80);
  }

  function flushConsole() {
    if (pendingLines.length > 0) {
      consoleLines = [...consoleLines, ...pendingLines];
      pendingLines = [];
      if (consoleLines.length > maxConsoleLines) consoleLines = consoleLines.slice(-keepConsoleLines);
      tick().then(() => { if (consoleEl) consoleEl.scrollTop = consoleEl.scrollHeight; });
    }
    flushTimer = null;
  }

  function clearConsole() {
    consoleLines = [];
    pendingLines = [];
  }

  onMount(() => {
    EventsOn('log', (msg) => addLine(msg));
    addLine('RLdev 2026 - Go édition');
    addLine('Prêt. Place les binaires dans ./bin : kprl16.exe, rlc2026.exe, vaconv.exe, rlxml.exe.');
  });

  onDestroy(() => { EventsOff('log'); });

  async function browseRlSeen() {
    const f = await SelectFile('Select SEEN.txt', '*.txt;*.TXT', 'SEEN archives');
    if (f) rlSeenFile = f;
  }
  async function browseRlSeenSave() {
    const f = await SelectSaveFile('Save SEEN.txt as', 'SEEN.TXT', '*.txt;*.TXT', 'SEEN archives');
    if (f) rlSeenFile = f;
  }
  async function browseRlOrg() {
    if (rlCompileBatch) {
      const d = await SelectDirectory('Select folder with .org / .ke files');
      if (d) rlOrgDir = d;
    } else {
      const f = await SelectFile('Select .org / .ke file', '*.org;*.ke', 'Kepago scripts');
      if (f) rlOrgFile = f;
    }
  }
  async function browseRlKfn() {
    const f = await SelectFile('Select .kfn file', '*.kfn', 'KFN files');
    if (f) rlKfnFile = f;
  }
  async function browseRlGameexe() {
    const f = await SelectFile('Select GAMEEXE.INI', '*.ini;*.INI', 'INI files');
    if (f) rlGameexe = f;
  }
  async function browseRlInterpreter() {
    const f = await SelectFile('Select RealLive.exe', '*.exe;*.EXE', 'RealLive interpreter');
    if (f) rlInterpreter = f;
  }
  async function browseRlOutputDir() {
    const d = await SelectDirectory('Select output directory');
    if (d) rlOutputDir = d;
  }
  async function browseRlG00() {
    const f = await SelectFile('Select .g00 file', '*.g00', 'G00 images');
    if (f) rlG00File = f;
  }
  async function browseRlPng() {
    const f = await SelectFile('Select .png file', '*.png', 'PNG images');
    if (f) rlPngFile = f;
  }
  async function browseRlGan() {
    const f = await SelectFile('Select .gan/.ganxml', '*.gan;*.ganxml', 'GAN files');
    if (f) rlGanFile = f;
  }

  async function run(fn) {
    if (running) return;
    running = true;
    try {
      await fn();
    } catch (e) {
      addLine('[ERROR] ' + e);
    }
    running = false;
  }

  function startRlDisasm() {
    run(() => RldevDisassemble(rlSeenFile, rlKfnFile, rlEncoding, rlGameId, rlOutputDir));
  }
  function startRlExtract() {
    run(() => RldevExtract(rlSeenFile, rlOutputDir));
  }
  function startRlArchive() {
    run(() => RldevArchive(rlSeenFile, rlOutputDir));
  }
  function startRlList() {
    run(() => RldevList(rlSeenFile));
  }
  function startRlCompile() {
    if (rlCompileBatch) {
      run(() => RldevCompileBatch(rlOrgDir, rlKfnFile, rlGameexe, rlInterpreter, rlEncoding, rlOutputTransform, rlForceTransform, rlOutputDir));
    } else {
      run(() => RldevCompile(rlOrgFile, rlKfnFile, rlGameexe, rlInterpreter, rlEncoding, rlOutputTransform, rlForceTransform, rlOutputDir));
    }
  }
  function toggleCompileBatch() {
    rlOrgFile = '';
    rlOrgDir = '';
  }
  function startG00Extract() {
    run(() => RldevG00ToPng(rlG00File, rlOutputDir));
  }
  function startG00Import() {
    run(() => RldevPngToG00(rlPngFile, rlOutputDir));
  }
  function startGanToXml() {
    run(() => RldevGanToXml(rlGanFile, rlOutputDir));
  }
  function startGanFromXml() {
    run(() => RldevXmlToGan(rlGanFile, rlOutputDir));
  }
  async function stopProcess() {
    await StopProcess();
  }
</script>

<div id="app">
  <div class="titlebar">
    <span>RLdev 2026 - Go édition</span>
    <span class="titlebar-path">Fork Yoremi · outils RealLive</span>
  </div>

  <div class="content">
    <div class="sidebar">
      <div class="sidebar-title">RLdev 2026 :</div>
      <div class="sidebar-list">
        {#each rldevOperations as op}
          {#if op.section}
            <div class="sidebar-section">{op.label}</div>
          {:else}
            <div class="sidebar-item" class:active={rldevSelectedOp === op.id} on:click={() => rldevSelectedOp = op.id}>
              {op.label}
            </div>
          {/if}
        {/each}
      </div>
    </div>

    <div class="form-panel">
      {#if rldevSelectedOp === 'kprl_disasm'}
        <div class="form-title">2 — Extract SEEN.txt</div>
        <div class="form-hint" style="margin-bottom:10px">Désassemble une archive SEEN.txt en scripts Kepago (.org + .utf/.sjs).</div>
        <div class="form-group"><label>SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeen}>Select</button></div></div>
        <div class="form-group"><label>KFN file (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlKfnFile} readonly placeholder="Auto : ./bin/lib/reallive.kfn" /><button class="btn" on:click={browseRlKfn}>Select</button></div></div>
        <div class="form-group"><label>Encodage sortie :</label><div class="form-row"><select bind:value={rlEncoding}><option value="UTF-8">UTF-8</option><option value="CP932">CP932 / Shift-JIS</option><option value="EUC-JP">EUC-JP</option></select></div></div>
        <div class="form-group"><label>Game ID (-G, optionnel) :</label><div class="form-row"><input type="text" bind:value={rlGameId} placeholder="ex: CLANNAD, KANON, AIR..." /></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlDisasm} disabled={!rlSeenFile || !rlOutputDir}>Start Extract</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_extract'}
        <div class="form-title">Extract raw / uncompressed</div>
        <div class="form-hint" style="margin-bottom:10px">Décompresse/décrypte les scénarios sans désassemblage.</div>
        <div class="form-group"><label>SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeen}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlExtract} disabled={!rlSeenFile || !rlOutputDir}>Extract</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_archive'}
        <div class="form-title">4 — Rebuild SEEN.txt</div>
        <div class="form-hint" style="margin-bottom:10px">Assemble des fichiers .TXT compilés dans une archive SEEN.txt.</div>
        <div class="form-group"><label>Input folder (*.TXT) :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-group"><label>Output SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeenSave}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlArchive} disabled={!rlSeenFile || !rlOutputDir}>Rebuild Archive</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_list'}
        <div class="form-title">1 — List SEEN.txt</div>
        <div class="form-group"><label>SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeen}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlList} disabled={!rlSeenFile}>List Contents</button>{/if}</div>

      {:else if rldevSelectedOp === 'rlc_compile'}
        <div class="form-title">3 — Compile .org / .ke → .TXT</div>
        <div class="form-hint" style="margin-bottom:10px">Compile un script Kepago, ou tous les .org/.ke d'un dossier en mode batch.</div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlCompileBatch} on:change={toggleCompileBatch} /> Batch mode</label></div></div>
        {#if rlCompileBatch}
          <div class="form-group"><label>Input folder (.org/.ke) :</label><div class="form-row"><input type="text" bind:value={rlOrgDir} readonly /><button class="btn" on:click={browseRlOrg}>Select</button></div></div>
        {:else}
          <div class="form-group"><label>Script .org / .ke :</label><div class="form-row"><input type="text" bind:value={rlOrgFile} readonly /><button class="btn" on:click={browseRlOrg}>Select</button></div></div>
        {/if}
        <div class="form-group"><label>KFN file (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlKfnFile} readonly placeholder="Auto : ./bin/lib/reallive.kfn" /><button class="btn" on:click={browseRlKfn}>Select</button></div></div>
        <div class="form-group"><label>GAMEEXE.INI (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlGameexe} readonly /><button class="btn" on:click={browseRlGameexe}>Select</button></div></div>
        <div class="form-group"><label>RealLive.exe (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlInterpreter} readonly /><button class="btn" on:click={browseRlInterpreter}>Select</button></div><div class="form-hint">Détection de version PE / overloads KFN si disponible.</div></div>
        <div class="form-group"><label>Encodage source :</label><div class="form-row"><select bind:value={rlEncoding}><option value="UTF-8">UTF-8</option><option value="CP932">CP932 / Shift-JIS</option><option value="EUC-JP">EUC-JP</option></select></div></div>
        <div class="form-group"><label>Transformation sortie :</label><div class="form-row"><select bind:value={rlOutputTransform}><option value="WESTERN">WESTERN / CP1252</option><option value="NONE">NONE / Japonais</option><option value="CHINESE">CHINESE</option><option value="KOREAN">KOREAN</option></select></div></div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlForceTransform} /> Force transform</label></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlCompile} disabled={(rlCompileBatch ? !rlOrgDir : !rlOrgFile) || !rlOutputDir}>Compile</button>{/if}</div>

      {:else if rldevSelectedOp === 'g00_extract'}
        <div class="form-title">G00 → PNG</div>
        <div class="form-group"><label>G00 file :</label><div class="form-row"><input type="text" bind:value={rlG00File} readonly /><button class="btn" on:click={browseRlG00}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startG00Extract} disabled={!rlG00File || !rlOutputDir}>Convert</button>{/if}</div>

      {:else if rldevSelectedOp === 'g00_import'}
        <div class="form-title">PNG → G00</div>
        <div class="form-group"><label>PNG file :</label><div class="form-row"><input type="text" bind:value={rlPngFile} readonly /><button class="btn" on:click={browseRlPng}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startG00Import} disabled={!rlPngFile || !rlOutputDir}>Convert</button>{/if}</div>

      {:else if rldevSelectedOp === 'gan_to_xml'}
        <div class="form-title">GAN → XML</div>
        <div class="form-group"><label>GAN file :</label><div class="form-row"><input type="text" bind:value={rlGanFile} readonly /><button class="btn" on:click={browseRlGan}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startGanToXml} disabled={!rlGanFile || !rlOutputDir}>Convert</button>{/if}</div>

      {:else if rldevSelectedOp === 'gan_from_xml'}
        <div class="form-title">XML → GAN</div>
        <div class="form-group"><label>GANXML file :</label><div class="form-row"><input type="text" bind:value={rlGanFile} readonly /><button class="btn" on:click={browseRlGan}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startGanFromXml} disabled={!rlGanFile || !rlOutputDir}>Convert</button>{/if}</div>
      {/if}
    </div>
  </div>

  <div class="console-wrapper">
    <div class="console-header">
      <span>Console</span>
      <div>
        {#if running}<button class="console-stop" on:click={stopProcess}>Stop</button>{/if}
        <button class="console-clear" on:click={clearConsole}>Clear</button>
      </div>
    </div>
    <div class="console" bind:this={consoleEl}>
      {#each consoleLines as line}
        <div class={line.cls}>{line.text}</div>
      {/each}
    </div>
  </div>
</div>
