<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime.js';
  import {
    SelectFile,
    SelectDirectory,
    SelectSaveFile,
    StopProcess,
    DefaultKFN,
    RldevDisassemble,
    RldevExtract,
    RldevArchive,
    RldevList,
    RldevCompile,
    RldevCompileBatch,
    RldevG00ToPng,
    RldevPngToG00,
    RldevGanToXml,
    RldevXmlToGan,
    RldevSaveInfo,
    RldevSaveMap,
    RldevSaveDoctor,
    RldevSaveDiff,
    RldevSaveGet,
    RldevSaveSet,
    RldevSaveDump,
    RldevSaveExport,
    RldevSaveBuild
  } from '../wailsjs/go/main/App.js';

  let rldevSelectedOp = 'kprl_disasm';
  let running = false;
  let consoleLines = [];
  let consoleEl;
  let consoleHeight = 160;
  let consoleResizing = false;
  let consoleResizeStartY = 0;
  let consoleResizeStartHeight = 160;

  let rlSeenFile = '';
  let rlTemplateSeenFile = '';
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
  let rlDebugInfo = false;
  let rlG00File = '';
  let rlG00Dir = '';
  let rlG00Batch = false;
  let rlG00XmlPath = '';
  let rlPngFile = '';
  let rlPngDir = '';
  let rlPngBatch = false;
  let rlPngXmlPath = '';
  let rlG00Format = 'auto';
  let rlGanFile = '';
  let rlSaveMapPath = '';
  let rlSaveFile = '';
  let rlSaveCompareFile = '';
  let rlSaveProfile = 'read_progress';
  let rlSaveRefs = 'seen[1] seen[100] dword[1]';
  let rlSaveAssignments = 'seen[1]=0';
  let rlSaveTextFile = '';
  let rlSaveBuildOutput = '';
  let rlSaveBackup = true;
  let rlSaveLossless = true;
  let rlSaveMapJson = false;
  let rlSaveDumpAll = false;
  let rlSaveDumpJson = false;
  let rlSaveDoctorJson = false;
  let rlSaveDiffJson = false;

  const rldevOperations = [
    { id: '_rs1', label: 'KPRL / RLC', section: true },
    { id: 'kprl_list',    label: '1 — List SEEN.txt' },
    { id: 'kprl_disasm',  label: '2 — Extract SEEN.txt' },
    { id: 'rlc_compile',  label: '3 — Compile .org / .ke / .avg' },
    { id: 'kprl_archive', label: '4 — Rebuild SEEN.txt' },
    { id: 'kprl_extract', label: 'Advanced: extract bytecode' },
    { id: '_rs3', label: 'IMAGE (G00)', section: true },
    { id: 'g00_extract', label: 'G00 → PNG' },
    { id: 'g00_import', label: 'PNG → G00' },
    { id: '_rs4', label: 'ANIMATION (GAN)', section: true },
    { id: 'gan_to_xml', label: 'GAN → XML' },
    { id: 'gan_from_xml', label: 'XML → GAN' },
    { id: '_rs_save', label: 'SAVE', section: true },
    { id: 'save_editor', label: 'RealLive save editor' }
  ];

  const saveProfiles = [
    {
      id: 'read_progress',
      label: 'read.sav progression',
      refs: 'seen[1] seen[100] seen[1000]',
      assignments: 'seen[1]=0'
    },
    {
      id: 'global_flags',
      label: 'save999 intG flags',
      refs: 'intG[0] intG[1] intG[30] intG[31]',
      assignments: 'intG[1]=0'
    },
    {
      id: 'raw_dwords',
      label: 'low-level dwords',
      refs: 'dword[0] dword[1] dword[2]',
      assignments: 'dword[1]=0'
    }
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

  onMount(async () => {
    EventsOn('log', (msg) => addLine(msg));
    addLine('RLdev 2026 - Go édition');
    addLine('Prêt. Place les binaires dans ./bin : kprl16, rlc2026, vaconv, rlxml, rlsave (ou .exe sous Windows).');
    const kfn = await DefaultKFN();
    if (kfn && !rlKfnFile) {
      rlKfnFile = kfn;
      addLine('KFN détecté : ' + kfn);
    }
  });

  onDestroy(() => {
    EventsOff('log');
    stopConsoleResize();
  });

  function startConsoleResize(event) {
    event.preventDefault();
    consoleResizing = true;
    consoleResizeStartY = event.clientY;
    consoleResizeStartHeight = consoleHeight;
    window.addEventListener('mousemove', resizeConsole);
    window.addEventListener('mouseup', stopConsoleResize);
  }

  function resizeConsole(event) {
    if (!consoleResizing) return;
    const maxHeight = Math.max(180, window.innerHeight - 130);
    const nextHeight = consoleResizeStartHeight + (consoleResizeStartY - event.clientY);
    consoleHeight = Math.min(maxHeight, Math.max(96, nextHeight));
  }

  function stopConsoleResize() {
    if (!consoleResizing) return;
    consoleResizing = false;
    window.removeEventListener('mousemove', resizeConsole);
    window.removeEventListener('mouseup', stopConsoleResize);
  }

  async function browseRlSeen() {
    const f = await SelectFile('Select SEEN.txt', '*.txt;*.TXT', 'SEEN archives');
    if (f) rlSeenFile = f;
  }
  async function browseRlSeenSave() {
    const f = await SelectSaveFile('Save SEEN.txt as', 'SEEN.TXT', '*.txt;*.TXT', 'SEEN archives');
    if (f) rlSeenFile = f;
  }
  async function browseRlTemplateSeen() {
    const f = await SelectFile('Select original/template SEEN.txt', '*.txt;*.TXT', 'SEEN archives');
    if (f) rlTemplateSeenFile = f;
  }
  async function browseRlOrg() {
    if (rlCompileBatch) {
      const d = await SelectDirectory('Select folder with .org / .ke / .avg files');
      if (d) rlOrgDir = d;
    } else {
      const f = await SelectFile('Select .org / .ke / .avg file', '*.org;*.ke;*.avg', 'RLdev scripts');
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
    const f = await SelectFile('Select RealLive / Steam .exe', '*.exe;*.EXE', 'RealLive-compatible interpreter');
    if (f) rlInterpreter = f;
  }
  async function browseRlOutputDir() {
    const d = await SelectDirectory('Select output directory');
    if (d) rlOutputDir = d;
  }
  async function browseRlG00() {
    if (rlG00Batch) {
      const d = await SelectDirectory('Select folder with .g00 files');
      if (d) rlG00Dir = d;
    } else {
      const f = await SelectFile('Select .g00 file', '*.g00;*.G00', 'G00 images');
      if (f) rlG00File = f;
    }
  }
  async function browseRlPng() {
    if (rlPngBatch) {
      const d = await SelectDirectory('Select folder with .png files');
      if (d) rlPngDir = d;
    } else {
      const f = await SelectFile('Select .png file', '*.png;*.PNG', 'PNG images');
      if (f) rlPngFile = f;
    }
  }
  async function browseRlG00Xml() {
    if (rlG00Batch) {
      const d = await SelectDirectory('Select XML output folder');
      if (d) rlG00XmlPath = d;
    } else {
      const f = await SelectSaveFile('Save metadata XML as', 'image.xml', '*.xml;*.XML', 'G00 metadata XML');
      if (f) rlG00XmlPath = f;
    }
  }
  async function browseRlPngXml() {
    if (rlPngBatch) {
      const d = await SelectDirectory('Select folder with .xml metadata files');
      if (d) rlPngXmlPath = d;
    } else {
      const f = await SelectFile('Select .xml metadata file', '*.xml;*.XML', 'G00 metadata XML');
      if (f) rlPngXmlPath = f;
    }
  }
  async function browseRlGan() {
    const f = await SelectFile('Select .gan/.ganxml', '*.gan;*.ganxml', 'GAN files');
    if (f) rlGanFile = f;
  }
  async function browseRlSave() {
    const f = await SelectFile('Select RealLive save', '*.sav;*.SAV', 'RealLive saves');
    if (f) rlSaveFile = f;
  }
  async function browseRlSaveCompare() {
    const f = await SelectFile('Select RealLive save to compare', '*.sav;*.SAV', 'RealLive saves');
    if (f) rlSaveCompareFile = f;
  }
  async function browseRlSaveMapFile() {
    const f = await SelectFile('Select RealLive save', '*.sav;*.SAV', 'RealLive saves');
    if (f) rlSaveMapPath = f;
  }
  async function browseRlSaveMapDir() {
    const d = await SelectDirectory('Select folder with RealLive saves');
    if (d) rlSaveMapPath = d;
  }
  async function browseRlSaveTextInput() {
    const f = await SelectFile('Select rlsave text export', '*.txt;*.TXT;*.rlsavetxt', 'rlsave text exports');
    if (f) rlSaveTextFile = f;
  }
  async function browseRlSaveTextOutput() {
    const f = await SelectSaveFile('Save rlsave text export as', 'save.txt', '*.txt;*.TXT;*.rlsavetxt', 'rlsave text exports');
    if (f) rlSaveTextFile = f;
  }
  async function browseRlSaveBuildOutput() {
    const f = await SelectSaveFile('Save rebuilt RealLive save as', 'rebuilt.sav', '*.sav;*.SAV', 'RealLive saves');
    if (f) rlSaveBuildOutput = f;
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
    run(() => RldevDisassemble(rlSeenFile, rlKfnFile, rlEncoding, rlGameId, rlDebugInfo, rlOutputDir));
  }
  function startRlExtract() {
    run(() => RldevExtract(rlSeenFile, rlOutputDir));
  }
  function startRlArchive() {
    run(() => RldevArchive(rlSeenFile, rlOutputDir, rlTemplateSeenFile));
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
  function toggleG00Batch() {
    rlG00File = '';
    rlG00Dir = '';
    rlG00XmlPath = '';
  }
  function togglePngBatch() {
    rlPngFile = '';
    rlPngDir = '';
    rlPngXmlPath = '';
  }
  function startG00Extract() {
    run(() => RldevG00ToPng(rlG00Batch ? rlG00Dir : rlG00File, rlOutputDir, rlG00XmlPath, rlG00Batch));
  }
  function startG00Import() {
    run(() => RldevPngToG00(rlPngBatch ? rlPngDir : rlPngFile, rlOutputDir, rlPngXmlPath, rlG00Format, rlPngBatch));
  }
  function startGanToXml() {
    run(() => RldevGanToXml(rlGanFile, rlOutputDir));
  }
  function startGanFromXml() {
    run(() => RldevXmlToGan(rlGanFile, rlOutputDir));
  }
  function startSaveInfo() {
    run(() => RldevSaveInfo(rlSaveFile));
  }
  function startSaveMap() {
    run(() => RldevSaveMap(rlSaveMapPath || rlSaveFile, rlSaveMapJson));
  }
  function startSaveDoctor() {
    run(() => RldevSaveDoctor(rlSaveMapPath || rlSaveFile, rlSaveDoctorJson));
  }
  function startSaveDiff() {
    run(() => RldevSaveDiff(rlSaveFile, rlSaveCompareFile, rlSaveDiffJson));
  }
  function startSaveGet() {
    run(() => RldevSaveGet(rlSaveFile, rlSaveRefs));
  }
  function startSaveSet() {
    run(() => RldevSaveSet(rlSaveFile, rlSaveAssignments, rlSaveBackup));
  }
  function startSaveDump() {
    run(() => RldevSaveDump(rlSaveFile, rlSaveDumpAll, rlSaveDumpJson));
  }
  function startSaveExport() {
    run(() => RldevSaveExport(rlSaveFile, rlSaveTextFile, rlSaveLossless));
  }
  function startSaveBuild() {
    run(() => RldevSaveBuild(rlSaveTextFile, rlSaveBuildOutput, rlSaveBackup));
  }
  function applySaveProfile() {
    const profile = saveProfiles.find((item) => item.id === rlSaveProfile);
    if (!profile) return;
    rlSaveRefs = profile.refs;
    rlSaveAssignments = profile.assignments;
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
        <div class="form-group"><label>KFN file :</label><div class="form-row"><input type="text" bind:value={rlKfnFile} readonly placeholder="Auto : ./KFN/reallive.kfn" /><button class="btn" on:click={browseRlKfn}>Select</button></div></div>
        <div class="form-group"><label>Encodage sortie :</label><div class="form-row"><select bind:value={rlEncoding}><option value="UTF-8">UTF-8</option><option value="CP932">CP932 / Shift-JIS</option><option value="EUC-JP">EUC-JP</option></select></div></div>
        <div class="form-group"><label>Game ID (-G, optionnel) :</label><div class="form-row"><input type="text" bind:value={rlGameId} placeholder="ex: CLANNAD, KANON, AIR..." /></div></div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlDebugInfo} /> Sources debug RealLive (-g / #line)</label></div><div class="form-hint">Pour F3/F5/O uniquement ; garder décoché pour les sources de traduction.</div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlDisasm} disabled={!rlSeenFile || !rlKfnFile || !rlOutputDir}>Start Extract</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_extract'}
        <div class="form-title">Advanced: extract bytecode</div>
        <div class="form-hint" style="margin-bottom:10px">Décompresse/décrypte les scénarios en fichiers .rl, sans produire de scripts .org.</div>
        <div class="form-group"><label>SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeen}>Select</button></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlExtract} disabled={!rlSeenFile || !rlOutputDir}>Extract</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_archive'}
        <div class="form-title">4 — Rebuild SEEN.txt</div>
        <div class="form-hint" style="margin-bottom:10px">Assemble des fichiers .TXT/.avg compilés dans une archive SEEN.txt.</div>
        <div class="form-group"><label>Input folder (.TXT/.avg) :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-group"><label>Original/template SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlTemplateSeenFile} readonly placeholder="Optionnel, requis pour Clannad Steam" /><button class="btn" on:click={browseRlTemplateSeen}>Select</button></div></div>
        <div class="form-group"><label>Output SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeenSave}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlArchive} disabled={!rlSeenFile || !rlOutputDir}>Rebuild Archive</button>{/if}</div>

      {:else if rldevSelectedOp === 'kprl_list'}
        <div class="form-title">1 — List SEEN.txt</div>
        <div class="form-group"><label>SEEN.txt :</label><div class="form-row"><input type="text" bind:value={rlSeenFile} readonly /><button class="btn" on:click={browseRlSeen}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlList} disabled={!rlSeenFile}>List Contents</button>{/if}</div>

      {:else if rldevSelectedOp === 'rlc_compile'}
        <div class="form-title">3 — Compile .org / .ke / .avg → .TXT</div>
        <div class="form-hint" style="margin-bottom:10px">Compile les scripts RealLive/Kepago ou AVG32 d'un dossier en mode batch.</div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlCompileBatch} on:change={toggleCompileBatch} /> Batch mode</label></div></div>
        {#if rlCompileBatch}
          <div class="form-group"><label>Input folder (.org/.ke/.avg) :</label><div class="form-row"><input type="text" bind:value={rlOrgDir} readonly /><button class="btn" on:click={browseRlOrg}>Select</button></div></div>
        {:else}
          <div class="form-group"><label>Script .org / .ke / .avg :</label><div class="form-row"><input type="text" bind:value={rlOrgFile} readonly /><button class="btn" on:click={browseRlOrg}>Select</button></div></div>
        {/if}
        <div class="form-group"><label>KFN file :</label><div class="form-row"><input type="text" bind:value={rlKfnFile} readonly placeholder="Auto : ./KFN/reallive.kfn" /><button class="btn" on:click={browseRlKfn}>Select</button></div></div>
        <div class="form-group"><label>GAMEEXE.INI (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlGameexe} readonly /><button class="btn" on:click={browseRlGameexe}>Select</button></div></div>
        <div class="form-group"><label>Interpréteur RealLive / Steam (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlInterpreter} readonly /><button class="btn" on:click={browseRlInterpreter}>Select</button></div><div class="form-hint">Auto si GAMEEXE.INI pointe vers un dossier contenant RealLive.exe ou SiglusEngine_Steam.exe.</div></div>
        <div class="form-group"><label>Encodage source :</label><div class="form-row"><select bind:value={rlEncoding}><option value="UTF-8">UTF-8</option><option value="CP932">CP932 / Shift-JIS</option><option value="EUC-JP">EUC-JP</option></select></div></div>
        <div class="form-group"><label>Transformation sortie :</label><div class="form-row"><select bind:value={rlOutputTransform}><option value="WESTERN">WESTERN / CP1252</option><option value="NONE">NONE / Japonais</option><option value="CHINESE">CHINESE</option><option value="KOREAN">KOREAN</option></select></div></div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlForceTransform} /> Force transform</label></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startRlCompile} disabled={(rlCompileBatch ? !rlOrgDir : !rlOrgFile) || !rlOutputDir}>Compile</button>{/if}</div>

      {:else if rldevSelectedOp === 'g00_extract'}
        <div class="form-title">G00 → PNG</div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlG00Batch} on:change={toggleG00Batch} /> Batch mode</label></div></div>
        {#if rlG00Batch}
          <div class="form-group"><label>G00 folder :</label><div class="form-row"><input type="text" bind:value={rlG00Dir} readonly /><button class="btn" on:click={browseRlG00}>Select</button></div></div>
          <div class="form-group"><label>XML folder (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlG00XmlPath} readonly placeholder="Auto : output folder" /><button class="btn" on:click={browseRlG00Xml}>Select</button></div></div>
        {:else}
          <div class="form-group"><label>G00 file :</label><div class="form-row"><input type="text" bind:value={rlG00File} readonly /><button class="btn" on:click={browseRlG00}>Select</button></div></div>
          <div class="form-group"><label>XML file (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlG00XmlPath} readonly placeholder="Auto : same output basename" /><button class="btn" on:click={browseRlG00Xml}>Select</button></div></div>
        {/if}
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startG00Extract} disabled={(rlG00Batch ? !rlG00Dir : !rlG00File) || !rlOutputDir}>Convert</button>{/if}</div>

      {:else if rldevSelectedOp === 'g00_import'}
        <div class="form-title">PNG → G00</div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlPngBatch} on:change={togglePngBatch} /> Batch mode</label></div></div>
        {#if rlPngBatch}
          <div class="form-group"><label>PNG folder :</label><div class="form-row"><input type="text" bind:value={rlPngDir} readonly /><button class="btn" on:click={browseRlPng}>Select</button></div></div>
          <div class="form-group"><label>XML folder (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlPngXmlPath} readonly placeholder="Auto : same PNG folder" /><button class="btn" on:click={browseRlPngXml}>Select</button></div></div>
        {:else}
          <div class="form-group"><label>PNG file :</label><div class="form-row"><input type="text" bind:value={rlPngFile} readonly /><button class="btn" on:click={browseRlPng}>Select</button></div></div>
          <div class="form-group"><label>XML file (optionnel) :</label><div class="form-row"><input type="text" bind:value={rlPngXmlPath} readonly placeholder="Auto : same PNG basename" /><button class="btn" on:click={browseRlPngXml}>Select</button></div></div>
        {/if}
        <div class="form-group"><label>G00 format :</label><div class="form-row"><select bind:value={rlG00Format}><option value="auto">Auto</option><option value="0">v0 simple</option><option value="1">v1 compressed</option><option value="2">v2 regions/XML</option></select></div></div>
        <div class="form-group"><label>Output folder :</label><div class="form-row"><input type="text" bind:value={rlOutputDir} readonly /><button class="btn" on:click={browseRlOutputDir}>Select</button></div></div>
        <div class="form-actions">{#if running}<span class="running-indicator"></span> Running...{:else}<button class="btn btn-primary" on:click={startG00Import} disabled={(rlPngBatch ? !rlPngDir : !rlPngFile) || !rlOutputDir}>Convert</button>{/if}</div>

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

      {:else if rldevSelectedOp === 'save_editor'}
        <div class="form-title">RealLive save editor</div>
        <div class="form-group"><label>Profile :</label><div class="form-row"><select bind:value={rlSaveProfile}>{#each saveProfiles as profile}<option value={profile.id}>{profile.label}</option>{/each}</select><button class="btn" on:click={applySaveProfile}>Apply</button></div></div>
        <div class="form-group"><label>Map target :</label><div class="form-row"><input type="text" bind:value={rlSaveMapPath} readonly placeholder="File or folder" /><button class="btn" on:click={browseRlSaveMapFile}>File</button><button class="btn" on:click={browseRlSaveMapDir}>Folder</button></div></div>
        <div class="form-group"><label>Save file :</label><div class="form-row"><input type="text" bind:value={rlSaveFile} readonly /><button class="btn" on:click={browseRlSave}>Select</button></div></div>
        <div class="form-group"><label>Compare with :</label><div class="form-row"><input type="text" bind:value={rlSaveCompareFile} readonly /><button class="btn" on:click={browseRlSaveCompare}>Select</button></div></div>
        <div class="form-group"><label>Variables :</label><div class="form-row"><input type="text" bind:value={rlSaveRefs} placeholder="intG[0] seen[100] dword[1]" /></div></div>
        <div class="form-group"><label>Assignations :</label><div class="form-row"><input type="text" bind:value={rlSaveAssignments} placeholder="intG[30]=0 seen[100]=0" /></div></div>
        <div class="form-group"><label>Text export :</label><div class="form-row"><input type="text" bind:value={rlSaveTextFile} readonly /><button class="btn" on:click={browseRlSaveTextOutput}>Export path</button><button class="btn" on:click={browseRlSaveTextInput}>Build input</button></div></div>
        <div class="form-group"><label>Rebuilt save :</label><div class="form-row"><input type="text" bind:value={rlSaveBuildOutput} readonly /><button class="btn" on:click={browseRlSaveBuildOutput}>Select</button></div></div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveBackup} /> Backup before write</label><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveLossless} /> Lossless export</label></div></div>
        <div class="form-group"><div class="form-row checkbox-row"><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveMapJson} /> Map JSON</label><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveDoctorJson} /> Doctor JSON</label><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveDiffJson} /> Diff JSON</label><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveDumpAll} /> Dump all intG values</label><label class="checkbox-label"><input type="checkbox" bind:checked={rlSaveDumpJson} /> Dump JSON</label></div></div>
        <div class="form-actions">
          {#if running}
            <span class="running-indicator"></span> Running...
          {:else}
            <button class="btn" on:click={startSaveMap} disabled={!rlSaveMapPath && !rlSaveFile}>Map</button>
            <button class="btn" on:click={startSaveDoctor} disabled={!rlSaveMapPath && !rlSaveFile}>Doctor</button>
            <button class="btn" on:click={startSaveInfo} disabled={!rlSaveFile}>Info</button>
            <button class="btn" on:click={startSaveDiff} disabled={!rlSaveFile || !rlSaveCompareFile}>Diff</button>
            <button class="btn" on:click={startSaveGet} disabled={!rlSaveFile || !rlSaveRefs.trim()}>Get</button>
            <button class="btn" on:click={startSaveDump} disabled={!rlSaveFile}>Dump</button>
            <button class="btn" on:click={startSaveExport} disabled={!rlSaveFile || !rlSaveTextFile}>Export</button>
            <button class="btn" on:click={startSaveBuild} disabled={!rlSaveTextFile || !rlSaveBuildOutput}>Build</button>
            <button class="btn btn-primary" on:click={startSaveSet} disabled={!rlSaveFile || !rlSaveAssignments.trim()}>Set</button>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <div class="console-wrapper" class:resizing={consoleResizing}>
    <div class="console-resizer" class:resizing={consoleResizing} on:mousedown={startConsoleResize}></div>
    <div class="console-header">
      <span>Console</span>
      <div>
        {#if running}<button class="console-stop" on:click={stopProcess}>Stop</button>{/if}
        <button class="console-clear" on:click={clearConsole}>Clear</button>
      </div>
    </div>
    <div class="console" bind:this={consoleEl} style:height={consoleHeight + 'px'}>
      {#each consoleLines as line}
        <div class={line.cls}>{line.text}</div>
      {/each}
    </div>
  </div>
</div>
