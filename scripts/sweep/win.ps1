#Requires -Version 5.1
# Windows helpers for the release sweep, for use on a disposable runner only:
# they list windows and processes and move the real mouse.
#
# Dot-source it, then call Invoke-Sweep with a JSON command. Each call
# prints one line of JSON. drive.mjs keeps one PowerShell open and sends it
# commands on stdin, so the C# below compiles only once.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.Windows.Forms, System.Drawing
if (-not ('SweepWin' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

public static class SweepWin {
    public delegate bool EnumProc(IntPtr h, IntPtr l);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc f, IntPtr l);
    [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int cmd);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr h, ref POINT p);
    [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool repaint);
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] static extern void mouse_event(uint f, int dx, int dy, uint d, UIntPtr e);
    [DllImport("user32.dll")] static extern void keybd_event(byte vk, byte scan, uint f, UIntPtr e);
    [DllImport("user32.dll")] static extern int GetSystemMetrics(int i);

    public struct RECT { public int Left, Top, Right, Bottom; }
    public struct POINT { public int X, Y; }

    [DllImport("user32.dll")] static extern bool IsHungAppWindow(IntPtr h);

    public class Win {
        public long Hwnd; public uint Pid; public string Title; public string Class;
        public bool Visible; public bool Iconic; public bool Hung; public int Left, Top, Right, Bottom;
    }

    public static Win Describe(IntPtr h) {
        uint pid; GetWindowThreadProcessId(h, out pid);
        var t = new StringBuilder(512); GetWindowText(h, t, 512);
        var c = new StringBuilder(256); GetClassName(h, c, 256);
        RECT r; GetWindowRect(h, out r);
        return new Win { Hwnd = h.ToInt64(), Pid = pid, Title = t.ToString(), Class = c.ToString(),
            Visible = IsWindowVisible(h), Iconic = IsIconic(h), Hung = IsHungAppWindow(h),
            Left = r.Left, Top = r.Top, Right = r.Right, Bottom = r.Bottom };
    }

    public static List<Win> Windows(uint[] pids) {
        var set = new HashSet<uint>(pids);
        var list = new List<Win>();
        EnumWindows((h, l) => {
            uint pid; GetWindowThreadProcessId(h, out pid);
            if (set.Contains(pid)) list.Add(Describe(h));
            return true;
        }, IntPtr.Zero);
        return list;
    }

    static void Abs(int x, int y, uint flags) {
        int w = GetSystemMetrics(0) - 1, hh = GetSystemMetrics(1) - 1;
        mouse_event(flags | 0x8000 | 0x0001, x * 65535 / w, y * 65535 / hh, 0, UIntPtr.Zero);
    }

    public static void Click(int x, int y) {
        Abs(x, y, 0); System.Threading.Thread.Sleep(80);
        Abs(x, y, 0x0002); System.Threading.Thread.Sleep(60);
        Abs(x, y, 0x0004);
    }

    // A slow drag with many small moves, as a hand would make, so OLE
    // drag and drop starts and the target sees the drag pass over it.
    public static void Drag(int x1, int y1, int x2, int y2, int steps) {
        Abs(x1, y1, 0); System.Threading.Thread.Sleep(200);
        Abs(x1, y1, 0x0002); System.Threading.Thread.Sleep(200);
        for (int i = 1; i <= 6; i++) { Abs(x1 + i * 2, y1 + i, 0); System.Threading.Thread.Sleep(40); }
        for (int i = 1; i <= steps; i++) {
            Abs(x1 + (x2 - x1) * i / steps, y1 + (y2 - y1) * i / steps, 0);
            System.Threading.Thread.Sleep(25);
        }
        System.Threading.Thread.Sleep(600);
        Abs(x2, y2, 0); System.Threading.Thread.Sleep(300);
        Abs(x2, y2, 0x0004);
    }

    // A drag in three parts, so a screenshot can be taken mid-drag.
    public static void Down(int x, int y) {
        Abs(x, y, 0); System.Threading.Thread.Sleep(200);
        Abs(x, y, 0x0002); System.Threading.Thread.Sleep(200);
        for (int i = 1; i <= 6; i++) { Abs(x + i * 2, y + i, 0); System.Threading.Thread.Sleep(40); }
    }
    public static void MoveSteps(int x1, int y1, int x2, int y2, int steps) {
        for (int i = 1; i <= steps; i++) {
            Abs(x1 + (x2 - x1) * i / steps, y1 + (y2 - y1) * i / steps, 0);
            System.Threading.Thread.Sleep(30);
        }
    }
    public static void Up(int x, int y) {
        Abs(x, y, 0); System.Threading.Thread.Sleep(300);
        Abs(x, y, 0x0004);
    }

    public static void Key(byte vk) {
        keybd_event(vk, 0, 0, UIntPtr.Zero); System.Threading.Thread.Sleep(30);
        keybd_event(vk, 0, 2, UIntPtr.Zero);
    }

    // A key with Ctrl held, e.g. Ctrl+A, Ctrl+V.
    public static void Ctrl(byte vk) {
        keybd_event(0x11, 0, 0, UIntPtr.Zero); System.Threading.Thread.Sleep(30);
        Key(vk);
        keybd_event(0x11, 0, 2, UIntPtr.Zero); System.Threading.Thread.Sleep(30);
    }

    public static void Wheel(int x, int y, int delta) {
        Abs(x, y, 0); System.Threading.Thread.Sleep(80);
        mouse_event(0x0800, 0, 0, unchecked((uint)delta), UIntPtr.Zero);
    }

    [DllImport("user32.dll")] static extern bool PostMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);
    // WM_CLOSE, as the title bar's close button sends.
    public static bool Close(IntPtr h) { return PostMessage(h, 0x0010, IntPtr.Zero, IntPtr.Zero); }

    [StructLayout(LayoutKind.Sequential)] struct KEYBDINPUT { public ushort wVk; public ushort wScan; public uint dwFlags; public uint time; public IntPtr dwExtraInfo; }
    [StructLayout(LayoutKind.Sequential)] struct MOUSEINPUT { public int dx; public int dy; public uint mouseData; public uint dwFlags; public uint time; public IntPtr dwExtraInfo; }
    [StructLayout(LayoutKind.Explicit)] struct InputUnion { [FieldOffset(0)] public MOUSEINPUT mi; [FieldOffset(0)] public KEYBDINPUT ki; }
    [StructLayout(LayoutKind.Sequential)] struct INPUT { public uint type; public InputUnion u; }
    [DllImport("user32.dll", SetLastError = true)] static extern uint SendInput(uint n, INPUT[] inputs, int size);

    // Types text into the focused control, one Unicode character at a time.
    public static void Type(string s) {
        foreach (char ch in s) {
            var down = new INPUT { type = 1 }; down.u.ki = new KEYBDINPUT { wScan = ch, dwFlags = 0x0004 };
            var up = new INPUT { type = 1 }; up.u.ki = new KEYBDINPUT { wScan = ch, dwFlags = 0x0004 | 0x0002 };
            SendInput(2, new[] { down, up }, Marshal.SizeOf(typeof(INPUT)));
            System.Threading.Thread.Sleep(8);
        }
    }

    // Windows lets a process take the foreground only in some cases. No
    // synthetic Alt here: in a window that already has focus, a lone Alt
    // starts keyboard menu mode, a modal loop. If Windows refuses, a real
    // click on the title bar does what a user's would.
    public static bool Focus(IntPtr h) {
        if (GetForegroundWindow() == h) return true;
        if (IsIconic(h)) ShowWindow(h, 9);
        SetForegroundWindow(h);
        System.Threading.Thread.Sleep(200);
        if (GetForegroundWindow() == h) return true;
        RECT r; GetWindowRect(h, out r);
        Click((r.Left + r.Right) / 2, r.Top + 10);
        System.Threading.Thread.Sleep(300);
        return GetForegroundWindow() == h;
    }
}
'@
}
[void][SweepWin]::SetProcessDPIAware()

$A = [System.Windows.Automation.AutomationElement]
$Scope = [System.Windows.Automation.TreeScope]

function Get-Pids([string]$Name) { @(Get-Process -Name $Name -ErrorAction SilentlyContinue | ForEach-Object { [uint32]$_.Id }) }

$IsWindow = New-Object System.Windows.Automation.PropertyCondition($A::ControlTypeProperty, [System.Windows.Automation.ControlType]::Window)

# Top-level windows plus the dialogs they own: UI Automation lists an owned
# window (a file dialog, a message box) under its owner, not the desktop.
function Get-Windows {
    foreach ($w in $A::RootElement.FindAll($Scope::Children, [System.Windows.Automation.Condition]::TrueCondition)) {
        $w
        foreach ($d in $w.FindAll($Scope::Children, $IsWindow)) { $d }
    }
}

function Find-Window([string]$TitleLike, [int]$TimeoutMs = 15000) {
    $end = [DateTime]::UtcNow.AddMilliseconds($TimeoutMs)
    do {
        foreach ($w in Get-Windows) {
            if ($w.Current.Name -like $TitleLike) { return $w }
        }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $end)
    return $null
}

function Find-Named($root, [string]$NameLike, [string]$Type = '') {
    # An exact name is matched by UI Automation itself, which is far faster.
    $cond = if ($NameLike -match '[*?\[]') { [System.Windows.Automation.Condition]::TrueCondition }
    else { New-Object System.Windows.Automation.PropertyCondition($A::NameProperty, $NameLike) }
    foreach ($e in $root.FindAll($Scope::Descendants, $cond)) {
        $c = $e.Current
        if ($c.Name -like $NameLike -and ($Type -eq '' -or $c.ControlType.ProgrammaticName -eq "ControlType.$Type")) { return $e }
    }
    return $null
}

# A browser window's own controls (tabs, toolbar, favorites bar, prompts),
# without the web page inside it: a job page's accessibility tree is huge.
function Get-ChromeElements($window) {
    $walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
    $stack = New-Object System.Collections.Stack
    $stack.Push($window)
    $n = 0
    while ($stack.Count -gt 0 -and $n -lt 3000) {
        $e = $stack.Pop()
        $n++
        $e
        if ($e.Current.ControlType -eq [System.Windows.Automation.ControlType]::Document) { continue }
        $child = $walker.GetFirstChild($e)
        while ($child) { $stack.Push($child); $child = $walker.GetNextSibling($child) }
    }
}

# A control in a row of a table: the row is found from its checkbox's name
# ("Select <company>, <role>"), then the control is looked for in the
# checkbox's ancestors, nearest first.
function Find-InRow($w, [string]$RowName, [string]$Name, [string]$Type) {
    $cb = Find-Named $w $RowName ''
    if (-not $cb) { return $null }
    $walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
    $isBox = New-Object System.Windows.Automation.PropertyCondition($A::ControlTypeProperty, [System.Windows.Automation.ControlType]::CheckBox)
    $p = $walker.GetParent($cb)
    # Climb to the row and never past it: an ancestor with a second checkbox
    # is the table, and its first match would belong to another row.
    while ($p -and $p.FindAll($Scope::Descendants, $isBox).Count -le 1) {
        $t = Find-Named $p $Name $Type
        if ($t) { return $t }
        $p = $walker.GetParent($p)
    }
    return $null
}

# Waits for a control in a window (optionally in a table row, optionally enabled).
function Find-Ui($c) {
    $w = Find-Window $c.window ([int]$c.timeout)
    if (-not $w) { throw "no window like '$($c.window)'" }
    $end = [DateTime]::UtcNow.AddMilliseconds([int]$c.timeout)
    $last = ''
    do {
        # A web page's accessibility tree is rebuilt as it changes, and a
        # search during a rebuild can time out; that's retried, not fatal.
        try {
            $e = if ($c.row) { Find-InRow $w $c.row $c.name ([string]$c.type) } else { Find-Named $w $c.name ([string]$c.type) }
            if ($e -and (-not $c.enabled -or $e.Current.IsEnabled)) { return $e }
        } catch { $last = " (last error: $($_.Exception.Message))" }
        Start-Sleep -Milliseconds 300
    } while ([DateTime]::UtcNow -lt $end)
    $where = if ($c.row) { " in row '$($c.row)'" } else { '' }
    throw "no $(if ($c.enabled) { 'enabled ' })'$($c.name)'$where in '$($w.Current.Name)'$last"
}

# An off-screen control has an empty rectangle, whose X and Y are infinite;
# JSON can't hold that, so it is reported as all zeros.
function Box-Of($b) {
    if ($b.IsEmpty) { return [ordered]@{ x = 0; y = 0; w = 0; h = 0 } }
    [ordered]@{ x = $b.X; y = $b.Y; w = $b.Width; h = $b.Height }
}

function Rect-Of($e) {
    $x = $e.Current
    $b = Box-Of $x.BoundingRectangle
    [ordered]@{ name = $x.Name; type = $x.ControlType.ProgrammaticName; x = $b.x; y = $b.y; w = $b.w; h = $b.h }
}

function Invoke-Element($e) {
    try {
        $e.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    } catch {
        $r = $e.Current.BoundingRectangle
        [SweepWin]::Click([int]($r.X + $r.Width / 2), [int]($r.Y + $r.Height / 2))
    }
}

function Text-Of($root) {
    ($root.FindAll($Scope::Descendants, [System.Windows.Automation.Condition]::TrueCondition) |
        ForEach-Object { $_.Current.Name } | Where-Object { $_ }) -join ' | '
}

function Save-Screen([string]$Path) {
    $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
    $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    try { $g.CopyFromScreen($b.Location, [System.Drawing.Point]::Empty, $b.Size) } finally { $g.Dispose() }
    $bmp.Save($Path); $bmp.Dispose()
}

function Invoke-Sweep([string]$Json) {
    # Commands leave out the fields they don't use (row, enabled, ...), which
    # strict mode would turn into errors. This applies inside this function only.
    Set-StrictMode -Off
    $c = $Json | ConvertFrom-Json
    $r = $null
    try {
        switch ($c.op) {
            'windows' {
                # A function returns an empty list as $null, so wrap it again here.
                $pids = [uint32[]]@(Get-Pids $c.name)
                $r = @([SweepWin]::Windows($pids) | Where-Object { $_.Visible -and $_.Title -ne '' })
            }
            'foreground' { $r = [SweepWin]::Describe([SweepWin]::GetForegroundWindow()) }
            'focus' { $r = [SweepWin]::Focus([IntPtr][long]$c.hwnd) }
            'minimize' { $r = [SweepWin]::ShowWindow([IntPtr][long]$c.hwnd, 6) }
            'move' {
                # A maximized window must be restored before it can be moved.
                [void][SweepWin]::ShowWindow([IntPtr][long]$c.hwnd, 9)
                $r = [SweepWin]::MoveWindow([IntPtr][long]$c.hwnd, $c.x, $c.y, $c.w, $c.h, $true)
            }
            'client' {
                $p = New-Object SweepWin+POINT
                [void][SweepWin]::ClientToScreen([IntPtr][long]$c.hwnd, [ref]$p)
                $r = @{ x = $p.X; y = $p.Y }
            }
            'click' { [SweepWin]::Click($c.x, $c.y); $r = $true }
            'drag' { [SweepWin]::Drag($c.x1, $c.y1, $c.x2, $c.y2, 40); $r = $true }
            'down' { [SweepWin]::Down($c.x, $c.y); $r = $true }
            'moveto' { [SweepWin]::MoveSteps($c.x1, $c.y1, $c.x2, $c.y2, [int]$c.steps); $r = $true }
            'up' { [SweepWin]::Up($c.x, $c.y); $r = $true }
            'kill-name' { Get-Process -Name $c.name -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue; $r = $true }
            'screen' { Save-Screen $c.path; $r = $c.path }
            'clipboard' { $r = [System.Windows.Forms.Clipboard]::GetText() }
            'pids' { $r = @(Get-Pids $c.name) }
            'dialogs' {
                # Windows and owned dialogs whose title matches, with their text and buttons.
                $r = @(foreach ($w in Get-Windows) {
                        $x = $w.Current
                        if ($x.Name -like $c.window -and $x.NativeWindowHandle -ne 0) {
                            $buttons = @($w.FindAll($Scope::Descendants, (New-Object System.Windows.Automation.PropertyCondition($A::ControlTypeProperty, [System.Windows.Automation.ControlType]::Button))) | ForEach-Object { $_.Current.Name })
                            [ordered]@{ hwnd = $x.NativeWindowHandle; title = $x.Name; text = (Text-Of $w); buttons = $buttons }
                        }
                    })
            }
            'press' {
                # A message box's buttons can sit in an owned window, and some
                # dialogs give them another control type.
                $w = $A::FromHandle([IntPtr][long]$c.hwnd)
                $b = Find-Named $w $c.button 'Button'
                if (-not $b) {
                    foreach ($d in $w.FindAll($Scope::Children, $IsWindow)) { $b = Find-Named $d $c.button 'Button'; if ($b) { break } }
                }
                if (-not $b) { $b = Find-Named $w $c.button '' }
                if (-not $b) { throw "no button '$($c.button)' in: $(Text-Of $w)" }
                # A press that worked closes the message box, and its button
                # goes with it. If the button is still there, a real click.
                $box = Box-Of $b.Current.BoundingRectangle
                Invoke-Element $b
                $gone = $false
                for ($i = 0; $i -lt 10 -and -not $gone; $i++) {
                    Start-Sleep -Milliseconds 200
                    try { $null = $b.Current.Name; $gone = -not $b.Current.IsEnabled -or $b.Current.IsOffscreen } catch { $gone = $true }
                }
                if (-not $gone -and $box.w -gt 0) {
                    [void][SweepWin]::Focus([IntPtr][long]$c.hwnd)
                    [SweepWin]::Click([int]($box.x + $box.w / 2), [int]($box.y + $box.h / 2))
                    for ($i = 0; $i -lt 10 -and -not $gone; $i++) {
                        Start-Sleep -Milliseconds 200
                        try { $null = $b.Current.Name } catch { $gone = $true }
                    }
                }
                $r = $gone
            }
            'ui' {
                # Finds a control in a window and acts on it like a user would.
                $e = Find-Ui $c
                switch ($c.action) {
                    'invoke' { Invoke-Element $e }
                    'toggle' { $e.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle() }
                    'focus' { $e.SetFocus() }
                    'scroll' { try { $e.GetCurrentPattern([System.Windows.Automation.ScrollItemPattern]::Pattern).ScrollIntoView() } catch {} }
                    default {}
                }
                $x = $e.Current
                $b = Box-Of $x.BoundingRectangle
                $value = $null
                try { $value = $e.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value } catch {}
                $toggle = $null
                try { $toggle = "$($e.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Current.ToggleState)" } catch {}
                $r = [ordered]@{ name = $x.Name; type = $x.ControlType.ProgrammaticName; enabled = $x.IsEnabled; x = $b.x; y = $b.y; w = $b.w; h = $b.h; value = $value; toggle = $toggle }
            }
            'ui-text' {
                $w = Find-Window $c.window ([int]$c.timeout)
                $r = ''
                if ($w) { try { $r = Text-Of $w } catch { $r = '' } }
            }
            'type' { [SweepWin]::Type([string]$c.text); $r = $true }
            'paste' {
                # Replaces the focused field's text through the clipboard.
                [System.Windows.Forms.Clipboard]::SetText([string]$c.text)
                [SweepWin]::Ctrl(0x41); Start-Sleep -Milliseconds 100
                [SweepWin]::Ctrl(0x56); Start-Sleep -Milliseconds 200
                $r = $true
            }
            'wheel' { [SweepWin]::Wheel($c.x, $c.y, $c.delta); $r = $true }
            'close' { $r = [SweepWin]::Close([IntPtr][long]$c.hwnd) }
            'chrome' {
                # A browser window's own controls whose name matches (see Get-ChromeElements).
                $w = Find-Window $c.window ([int]$c.timeout)
                if (-not $w) { throw "no window like '$($c.window)'" }
                $r = @(Get-ChromeElements $w | Where-Object {
                        $_.Current.Name -like $c.name -and ($c.type -eq '' -or $_.Current.ControlType.ProgrammaticName -eq "ControlType.$($c.type)")
                    } | ForEach-Object { Rect-Of $_ })
            }
            'filedialog' {
                # Windows' open-file dialog opens with the caret in File name,
                # so the path is typed there and Enter opens it, as a user would.
                $w = Find-Window $c.window ([int]$c.timeout)
                if (-not $w) { throw "no file dialog like '$($c.window)'; windows: $((Get-Windows | ForEach-Object { $_.Current.Name }) -join ' | ')" }
                $hwnd = [IntPtr][long]$w.Current.NativeWindowHandle
                if (-not [SweepWin]::Focus($hwnd)) { throw "the file dialog could not be brought to the front" }
                Start-Sleep -Milliseconds 300
                [SweepWin]::Ctrl(0x41)
                [SweepWin]::Type([string]$c.path)
                Start-Sleep -Milliseconds 300
                [SweepWin]::Key(0x0D)
                $gone = $false
                for ($i = 0; $i -lt 20 -and -not $gone; $i++) {
                    Start-Sleep -Milliseconds 250
                    $gone = -not (Get-Windows | Where-Object { $_.Current.NativeWindowHandle -eq $hwnd.ToInt64() })
                }
                if (-not $gone) { throw "the file dialog is still open after Enter: $(Text-Of $w)" }
                $r = $true
            }
            'procs' {
                $r = @(Get-CimInstance Win32_Process -Filter "Name='$($c.name)'" | ForEach-Object {
                        [ordered]@{ pid = $_.ProcessId; ppid = $_.ParentProcessId; cmd = $_.CommandLine }
                    })
            }
            'kill' { foreach ($id in @($c.pids)) { Stop-Process -Id $id -Force -ErrorAction SilentlyContinue }; $r = $true }
            'watch-starts' {
                Get-EventSubscriber -SourceIdentifier sweepStarts -ErrorAction SilentlyContinue | Unregister-Event
                Register-CimIndicationEvent -ClassName Win32_ProcessStartTrace -SourceIdentifier sweepStarts
                $r = $true
            }
            'starts' {
                # Processes started since the last call (needs admin).
                $r = @(Get-Event -SourceIdentifier sweepStarts -ErrorAction SilentlyContinue | ForEach-Object {
                        $e = $_.SourceEventArgs.NewEvent
                        [ordered]@{ name = $e.ProcessName; pid = $e.ProcessID; ppid = $e.ParentProcessID }
                        Remove-Event -EventIdentifier $_.EventIdentifier
                    })
            }
            'run' {
                # Windows PowerShell turns a native command's stderr into an
                # error under 'Stop', so this one call runs under 'Continue'.
                $ErrorActionPreference = 'Continue'
                try {
                    $o = & $c.file @($c.args) 2>&1 | ForEach-Object { "$_" }
                    $r = [ordered]@{ code = $LASTEXITCODE; output = ($o -join "`n") }
                } finally { $ErrorActionPreference = 'Stop' }
            }
            'shell' {
                # Opens a file or shortcut through Explorer, as a double-click
                # would: it runs with the desktop's ordinary (non-elevated)
                # rights, not the runner's elevated ones.
                Start-Process -FilePath 'explorer.exe' -ArgumentList "`"$($c.path)`""
                $r = $true
            }
            'shortcut' {
                $s = (New-Object -ComObject WScript.Shell).CreateShortcut($c.path)
                $s.TargetPath = $c.target
                $s.Arguments = $c.arguments
                if ($c.windowStyle) { $s.WindowStyle = [int]$c.windowStyle }
                $s.Save()
                $r = $c.path
            }
            'regvalue' {
                $v = Get-ItemProperty -LiteralPath $c.key -Name $c.name -ErrorAction SilentlyContinue
                $r = if ($v) { "$($v.($c.name))" } else { $null }
            }
            'set-assoc' {
                # A per-user handler for an extension, so ShellExecute can open it.
                $prog = "Sweep$($c.ext.TrimStart('.'))"
                New-Item -Path "HKCU:\Software\Classes\$($c.ext)" -Force | Out-Null
                Set-ItemProperty -Path "HKCU:\Software\Classes\$($c.ext)" -Name '(default)' -Value $prog
                New-Item -Path "HKCU:\Software\Classes\$prog\shell\open\command" -Force | Out-Null
                Set-ItemProperty -Path "HKCU:\Software\Classes\$prog\shell\open\command" -Name '(default)' -Value "`"$($c.exe)`" `"%1`""
                $r = $prog
            }
            default { throw "unknown op $($c.op)" }
        }
        [ordered]@{ ok = $true; value = $r } | ConvertTo-Json -Compress -Depth 6
    } catch {
        [ordered]@{ ok = $false; error = "$($_.Exception.Message)" } | ConvertTo-Json -Compress
    }
}
