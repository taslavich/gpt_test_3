<?php /* Smarty version 2.6.18, created on 2025-12-20 07:31:49
         compiled from maintenance-security.html */ ?>
<?php require_once(SMARTY_CORE_DIR . 'core.load_plugins.php');
smarty_core_load_plugins(array('plugins' => array(array('function', 't', 'maintenance-security.html', 14, false),array('function', 'phpAds_ShowBreak', 'maintenance-security.html', 16, false),array('modifier', 'escape', 'maintenance-security.html', 19, false),)), $this); ?>
<br />
<p>
  <?php echo $this->_plugins['function']['t'][0](['str' => 'SecurityExplanation'], $this);?>

</p>
<?php echo $this->_plugins['function']['phpAds_ShowBreak'][0]([], $this);?>

<div id="security-ok" style="display: none">
  <p>
    <img src="<?php echo ((is_array($_tmp=$this->_tpl_vars['assetPath'])) ? $this->_run_mod_handler('escape', true, $_tmp) : smarty_modifier_escape($_tmp)); ?>
/images/info.gif" align="top"/>&nbsp;&nbsp;<?php echo $this->_plugins['function']['t'][0](['str' => 'SecurityOK'], $this);?>

  </p>
</div>

<div id="security-ko" style="display: none">
  <p>
    <img src="<?php echo ((is_array($_tmp=$this->_tpl_vars['assetPath'])) ? $this->_run_mod_handler('escape', true, $_tmp) : smarty_modifier_escape($_tmp)); ?>
/images/error.gif" align="top"/>&nbsp;&nbsp;<?php echo $this->_plugins['function']['t'][0](['str' => 'SecurityKO'], $this);?>

  </p>
  <ul style="list-style: disc inside"></ul>
  <p></p>
  <p>
    <a href="<?php echo ((is_array($_tmp=$this->_tpl_vars['securityHowTo'])) ? $this->_run_mod_handler('escape', true, $_tmp) : smarty_modifier_escape($_tmp)); ?>
" target="_blank"><?php echo $this->_plugins['function']['t'][0](['str' => 'SecurityReadMore'], $this);?>
</a>
  </p>
</div>

<script>
  // <?php echo '
  RV_securityCheck(\''; ?>
<?php echo ((is_array($_tmp=$this->_tpl_vars['adminPath'])) ? $this->_run_mod_handler('escape', true, $_tmp, 'js') : smarty_modifier_escape($_tmp, 'js')); ?>
<?php echo '\', function (e) {
    if (e.length) {
      $(e).each(function (index, error) {
        $(\'#security-ko ul\').append($("<li>").text(error));
      });

      $(\'#security-ko\').show();
    } else {
      $(\'#security-ok\').show();
    }
  });
  // '; ?>

</script>