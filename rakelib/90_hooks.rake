# Hooks
# The file contains tasks to write and build the Stork hook libraries
# (GO plugins)

#############
### Files ###
#############

default_hook_directory_rel = "hooks"
DEFAULT_HOOK_DIRECTORY = File.expand_path default_hook_directory_rel

CLEAN.append *FileList[File.join(DEFAULT_HOOK_DIRECTORY, "*.so")]


#################
### Functions ###
#################

# Iterates over the hook directories and executes the given block for each
# of them.
# The block may accept three arguments:
#
# 1. Hook directory name
# 2. Absolute path to the hook directory
# 3. Absolute path the the subdirectory in the hook directory containing the
#    go.mod file (source subdirectory).
#
# The current working directory during the block execution is the subdirectory
# containing the go.mod file.
def forEachHook(&block)
    require 'find'

    hook_directory = ENV["HOOK_DIR"] || DEFAULT_HOOK_DIRECTORY

    Dir.foreach(hook_directory) do |dir_name|
        path = File.join(hook_directory, dir_name)
        next if dir_name == '.' or dir_name == '..' or !File.directory? path

        Dir.chdir(path) do
            project_path = File.expand_path path

            # Search for the go.mod
            src_path = nil

            Find.find '.' do |path|
                if File.basename(path) == 'go.mod'
                    src_path = File.dirname(path)
                    break
                end
            end

            if src_path.nil?
                fail 'Cannot find the go.mod file'
            end

            src_path = File.expand_path src_path

            Dir.chdir(src_path) do
                block.call(dir_name, project_path, src_path)
            end
        end
    end
end

#############
### Tasks ###
#############

namespace :hook do
    desc "Init new hook directory
        MODULE - the name  of the hook module used in the go.mod file and as the hook directory name - required
        HOOK_DIR - the directory containing the hooks - optional, default: #{default_hook_directory_rel}"
    task :init => [GO] do
        module_name = ENV["MODULE"]
        if module_name.nil?
            fail "You must provide the MODULE variable with the module name"
        end

        hook_directory = ENV["HOOK_DIR"] || DEFAULT_HOOK_DIRECTORY
        
        module_directory_name = module_name.gsub(/[^\w\.-]/, '_')

        destination = File.expand_path(File.join(hook_directory, module_directory_name))

        require 'pathname'
        main_module = "isc.org/stork@v0.0.0"
        main_module_directory_abs = Pathname.new('backend').realdirpath
        module_directory_abs = Pathname.new(destination)
        module_directory_rel = main_module_directory_abs.relative_path_from module_directory_abs

        sh "mkdir", "-p", destination

        Dir.chdir(destination) do
            sh "git", "init"
            sh GO, "mod", "init", module_name
            sh GO, "mod", "edit", "-require", main_module
            sh GO, "mod", "edit", "-replace", "#{main_module}=#{module_directory_rel}"
            sh "touch", "go.sum"
        end
        
        sh "cp", *FileList["backend/hooksutil/boilerplate/*"], destination
    end

    desc "Build all hooks. Remap hooks to use the current codebase.
        DEBUG - build hooks in debug mode, the envvar is passed through to the hook Rakefile - default: false
        HOOK_DIR - the hook (plugin) directory - optional, default: #{default_hook_directory_rel}"
    task :build => [GO, :remap_core] do
        require 'tmpdir'

        hook_directory = ENV["HOOK_DIR"] || DEFAULT_HOOK_DIRECTORY

        # Removes old hooks
        puts "Removing old compiled hooks..."
        sh "rm", "-f", *FileList[File.join(hook_directory, "*.so")]

        mod_files = ["go.mod", "go.sum"]

        forEachHook do |dir_name, project_path|
            # Make a backup of the original mod files
            Dir.mktmpdir do |temp|
                sh "cp", *mod_files, temp

                puts "Building #{dir_name}..."
                sh "rake", "build"

                sh "cp", *FileList[File.join(project_path, "build/*.so")], hook_directory

                # Back the changes in Go mod files.
                puts "Reverting remap operation..."
                sh "cp", *mod_files.collect { |f| File.join(temp, f) }, "."
            end
        end
    end

    desc "Lint hooks against the Stork core rules.
        FIX - fix linting issues - default: false
        HOOK_DIR - the directory containing the hooks - optional, default: #{default_hook_directory_rel}"
    task :lint => [GOLANGCILINT] do
        require 'pathname'

        opts = []
        if ENV["FIX"] == "true"
            opts += ["--fix"]
        end

        # Use relative path for more human-friendly linter output.
        hook_directory = Pathname.new(ENV["HOOK_DIR"] || DEFAULT_HOOK_DIRECTORY)
        main_directory = Pathname.new Dir.pwd
        hook_directory_rel = hook_directory.relative_path_from main_directory
        config_path = File.expand_path "backend/.golangci.yml"

        forEachHook do |dir_name|
            sh GOLANGCILINT, "run",
                "-c",  config_path,
                "--path-prefix", File.join(hook_directory_rel, dir_name),
                *opts
        end
    end

    desc "Remap the dependency path to the Stork core. It specifies the source
        of the core dependency - remote repository or local directory. The
        remote repository may be fetched by tag or commit hash.
        HOOK_DIR - the hook (plugin) directory - optional, default: #{default_hook_directory_rel}
        COMMIT - use the given commit from the remote repository, if specified but empty use the current hash - optional
        TAG - use the given tag from the remote repository, if specified but empty use the current version as tag - optional
        If no COMMIT or TAG are specified then it remaps to use the local project."
    task :remap_core => [GO] do
        main_module = "isc.org/stork"
        main_module_directory_abs = File.expand_path "backend"
        remote_url = "gitlab.isc.org/isc-projects/stork/backend"
        core_commit, _ = Open3.capture2 "git", "rev-parse", "HEAD"

        forEachHook do |dir_name|
            target = nil

            if !ENV["COMMIT"].nil?
                puts "Remap to use a specific commit"
                commit = ENV["COMMIT"]
                if commit == ""
                    commit = core_commit
                end

                target = "#{remote_url}@#{commit}"
            elsif !ENV["TAG"].nil?
                puts "Remap to use a specific tag"
                tag = ENV["TAG"]
                if tag == ""
                    tag = STORK_VERSION
                end

                if !tag.start_with? "v"
                    tag = "v" + tag
                end

                target = "#{remote_url}@#{tag}"
            else
                puts "Remap to use the local directory"
                require 'pathname'
                main_directory_abs_obj = Pathname.new(main_module_directory_abs)
                module_directory_abs_obj = Pathname.new(".").realdirpath
                module_directory_rel_obj = main_directory_abs_obj.relative_path_from module_directory_abs_obj

                target = module_directory_rel_obj.to_s
            end

            sh GO, "mod", "edit", "-replace", "#{main_module}=#{target}"
            sh GO, "mod", "tidy"
        end
    end

    desc "List dependencies of a given callout specification package
        KIND - callout kind - required, choice: agent or server
        CALLOUT - callout specification (interface) package name - required"
    task :list_callout_deps => [GO] do
        kind = ENV["KIND"]
        if kind != "server" && kind != "agent"
            fail "You need to provide the callout kind in KIND variable: agent or server"
        end

        callout = ENV["CALLOUT"]
        if callout.nil?
            fail "You need to provide the callout package name in CALLOUT variable."
        end

        package_rel = "hooks/#{kind}/#{callout}"
        ENV["REL"] = package_rel
        Rake::Task["utils:list_package_deps"].invoke
    end
end

namespace :run do
    desc "Run Stork Server with hooks
        HOOK_DIR - the hook (plugin) directory - optional, default: #{default_hook_directory_rel}"
    task :server_hooks => ["hook:build"] do
        hook_directory = ENV["HOOK_DIR"] || ENV["STORK_SERVER_HOOK_DIRECTORY"] || DEFAULT_HOOK_DIRECTORY
        ENV["STORK_SERVER_HOOK_DIRECTORY"] = hook_directory
        Rake::Task["run:server"].invoke()
    end
end
